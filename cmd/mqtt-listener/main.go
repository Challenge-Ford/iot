package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"torque-iot/internal/core/logger"
)

const (
	serviceClientID = "torque-listener"

	queueVehicleStateObserved = "torque.vehicle.state.observed"
	stateTopic                = "torque/vehicles/+/state"
)

type mqttStateSnapshot struct {
	ObservedAt  *time.Time     `json:"observed_at"`
	State       map[string]any `json:"state"`
	Observation map[string]any `json:"observation"`
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing required environment variable: %s\n", key)
		os.Exit(1)
	}
	return v
}

func main() {
	godotenv.Load()

	log, err := logger.New(os.Getenv("LOG_JSON") == "true")
	if err != nil {
		fmt.Println("failed to init logger:", err)
		os.Exit(1)
	}
	defer log.Sync()

	db, err := sql.Open("postgres", mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal("failed to open database", zap.Error(err))
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}

	tlsConfig, err := buildTLSConfig(
		mustEnv("MQTT_CA_CERT"),
		mustEnv("MQTT_CERT"),
		mustEnv("MQTT_KEY"),
		os.Getenv("MQTT_SERVER_NAME"),
	)
	if err != nil {
		log.Fatal("failed to build tls config", zap.Error(err))
	}

	amqpConn, err := amqp.Dial(mustEnv("RABBITMQ_URL"))
	if err != nil {
		log.Fatal("failed to connect to rabbitmq", zap.Error(err))
	}
	defer amqpConn.Close()

	ch, err := amqpConn.Channel()
	if err != nil {
		log.Fatal("failed to open rabbitmq channel", zap.Error(err))
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(queueVehicleStateObserved, true, false, false, false, nil); err != nil {
		log.Fatal("failed to declare queue", zap.String("queue", queueVehicleStateObserved), zap.Error(err))
	}

	brokerURL := mustEnv("MQTT_BROKER_URL")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(serviceClientID)
	opts.SetTLSConfig(tlsConfig)
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Info("connected to mqtt broker", zap.String("broker", brokerURL))
		var mu sync.Mutex
		token := c.Subscribe(stateTopic, 1, makeHandler(log, db, ch, &mu))
		if token.Wait() && token.Error() != nil {
			log.Error("failed to subscribe", zap.String("topic", stateTopic), zap.Error(token.Error()))
		} else {
			log.Info("subscribed", zap.String("topic", stateTopic))
		}
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Warn("mqtt connection lost", zap.Error(err))
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		log.Fatal("failed to connect to mqtt broker", zap.Error(token.Error()))
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Info("shutting down")
	client.Disconnect(500)
}

func makeHandler(log *zap.Logger, db *sql.DB, ch *amqp.Channel, mu *sync.Mutex) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		vehicleID := vehicleIDFromStateTopic(msg.Topic())
		if vehicleID == "" {
			log.Warn("could not extract vehicle id from topic", zap.String("topic", msg.Topic()))
			return
		}

		deviceID, err := deviceIDByVehicleID(context.Background(), db, vehicleID)
		if err != nil {
			log.Error("failed to resolve device by vehicle", zap.String("vehicle_id", vehicleID), zap.Error(err))
			return
		}

		items, err := unpackPayload(msg.Payload())
		if err != nil {
			log.Error("failed to unpack payload", zap.String("topic", msg.Topic()), zap.Error(err))
			return
		}

		forwarded := 0
		for i, item := range items {
			body, err := buildBackendMessage(deviceID, vehicleID, item)
			if err != nil {
				log.Warn("invalid state snapshot", zap.String("topic", msg.Topic()), zap.Int("index", i), zap.Error(err))
				continue
			}

			mu.Lock()
			err = ch.PublishWithContext(context.Background(), "", queueVehicleStateObserved, false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
				Body:         body,
			})
			mu.Unlock()
			if err != nil {
				log.Error("failed to publish to rabbitmq", zap.String("queue", queueVehicleStateObserved), zap.Error(err))
				continue
			}
			forwarded++
		}

		log.Debug("forwarded state snapshots",
			zap.String("topic", msg.Topic()),
			zap.String("queue", queueVehicleStateObserved),
			zap.Int("count", forwarded),
		)
	}
}

func deviceIDByVehicleID(ctx context.Context, db *sql.DB, vehicleID string) (string, error) {
	var deviceID string
	err := db.QueryRowContext(ctx, `
		SELECT d.id::text
		FROM device.devices d
		JOIN vehicle.vehicles v ON v.id = d.vehicle_id
		WHERE d.vehicle_id = $1
		  AND d.deleted_at IS NULL
		  AND v.deleted_at IS NULL
		LIMIT 1`, vehicleID).Scan(&deviceID)
	if err != nil {
		return "", err
	}
	return deviceID, nil
}

func vehicleIDFromStateTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) != 4 || parts[0] != "torque" || parts[1] != "vehicles" || parts[3] != "state" {
		return ""
	}
	return parts[2]
}

func unpackPayload(payload []byte) ([]mqttStateSnapshot, error) {
	var arr []mqttStateSnapshot
	if err := json.Unmarshal(payload, &arr); err == nil {
		return arr, nil
	}
	var obj mqttStateSnapshot
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, err
	}
	return []mqttStateSnapshot{obj}, nil
}

func buildBackendMessage(deviceID, vehicleID string, item mqttStateSnapshot) ([]byte, error) {
	if item.ObservedAt == nil || item.ObservedAt.IsZero() {
		return nil, fmt.Errorf("observed_at is required")
	}
	if len(item.State) == 0 {
		return nil, fmt.Errorf("state is required")
	}
	observation := item.Observation
	if observation == nil {
		observation = map[string]any{"errors": []any{}}
	}

	body := map[string]any{
		"schema_version": 1,
		"message_id":     newUUIDString(),
		"device_id":      deviceID,
		"vehicle_id":     vehicleID,
		"observed_at":    item.ObservedAt.UTC().Format(time.RFC3339Nano),
		"state":          item.State,
		"observation":    observation,
	}
	return json.Marshal(body)
}

func newUUIDString() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func buildTLSConfig(caCertPath, certPath, keyPath, serverName string) (*tls.Config, error) {
	caPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read ca cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse ca certificate")
	}

	clientCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	cfg := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
	}
	if serverName != "" {
		cfg.ServerName = serverName
	}
	return cfg, nil
}
