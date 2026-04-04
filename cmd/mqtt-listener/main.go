package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"torque-iot/internal/core/logger"
	"torque-iot/internal/core/pki"
)

const (
	serviceClientID = "torque-listener"
	serviceCN       = "torque-listener"
	vaultPKIRole    = "service"

	queueTelemetry = "torque.telemetry"
	queueDTC       = "torque.dtc"
	queueSession   = "torque.session"
)

var topicQueues = map[string]string{
	"telemetry": queueTelemetry,
	"dtc":       queueDTC,
	"session":   queueSession,
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

	// --- Vault PKI ---
	vaultPKI, err := pki.NewVaultPKI(mustEnv("VAULT_ADDR"), mustEnv("VAULT_TOKEN"), vaultPKIRole)
	if err != nil {
		log.Fatal("failed to init vault pki", zap.Error(err))
	}

	ctx := context.Background()

	log.Info("issuing service certificate", zap.String("cn", serviceCN))
	cert, err := vaultPKI.Issue(ctx, serviceCN)
	if err != nil {
		log.Fatal("failed to issue certificate", zap.Error(err))
	}

	caCertPEM, err := vaultPKI.FetchCACert(ctx)
	if err != nil {
		log.Fatal("failed to fetch ca cert", zap.Error(err))
	}

	mqttServerName := os.Getenv("MQTT_SERVER_NAME")
	if mqttServerName == "" {
		mqttServerName = "emqx"
	}

	tlsConfig, err := buildTLSConfig(caCertPEM, cert.Certificate, cert.PrivateKey, mqttServerName)
	if err != nil {
		log.Fatal("failed to build tls config", zap.Error(err))
	}

	// --- RabbitMQ ---
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

	for _, queue := range []string{queueTelemetry, queueDTC, queueSession} {
		if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
			log.Fatal("failed to declare queue", zap.String("queue", queue), zap.Error(err))
		}
	}

	// --- MQTT ---
	brokerURL := mustEnv("MQTT_BROKER_URL")

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID(serviceClientID)
	opts.SetTLSConfig(tlsConfig)
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Info("connected to mqtt broker", zap.String("broker", brokerURL))
		for suffix := range topicQueues {
			topic := "torque/vehicles/+/" + suffix
			token := c.Subscribe(topic, 1, makeHandler(log, ch, suffix))
			if token.Wait() && token.Error() != nil {
				log.Error("failed to subscribe", zap.String("topic", topic), zap.Error(token.Error()))
			} else {
				log.Info("subscribed", zap.String("topic", topic))
			}
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

// makeHandler returns an MQTT message handler that unpacks the payload (single
// object or array of objects), injects the VIN into each item, and publishes
// them individually to the appropriate RabbitMQ queue.
func makeHandler(log *zap.Logger, ch *amqp.Channel, suffix string) mqtt.MessageHandler {
	queue := topicQueues[suffix]
	return func(_ mqtt.Client, msg mqtt.Message) {
		vin := vinFromTopic(msg.Topic())
		if vin == "" {
			log.Warn("could not extract vin from topic", zap.String("topic", msg.Topic()))
			return
		}

		items, err := unpackPayload(msg.Payload())
		if err != nil {
			log.Error("failed to unpack payload", zap.String("topic", msg.Topic()), zap.Error(err))
			return
		}

		for _, item := range items {
			item["vin"] = vin
			body, err := json.Marshal(item)
			if err != nil {
				log.Error("failed to marshal item", zap.Error(err))
				continue
			}

			if err := ch.PublishWithContext(context.Background(), "", queue, false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
				Body:         body,
			}); err != nil {
				log.Error("failed to publish to rabbitmq", zap.String("queue", queue), zap.Error(err))
			}
		}

		log.Debug("forwarded messages", zap.String("topic", msg.Topic()), zap.String("queue", queue), zap.Int("count", len(items)))
	}
}

// vinFromTopic extracts the VIN from topics like torque/vehicles/{vin}/{suffix}.
func vinFromTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) != 4 {
		return ""
	}
	return parts[2]
}

// unpackPayload accepts either a JSON object or a JSON array and always returns
// a slice of map[string]any so callers can iterate uniformly.
func unpackPayload(payload []byte) ([]map[string]any, error) {
	// Try array first.
	var arr []map[string]any
	if err := json.Unmarshal(payload, &arr); err == nil {
		return arr, nil
	}

	// Fall back to single object.
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, err
	}
	return []map[string]any{obj}, nil
}

func buildTLSConfig(caCertPEM, certPEM, keyPEM, serverName string) (*tls.Config, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(caCertPEM)) {
		return nil, fmt.Errorf("failed to parse ca certificate")
	}

	clientCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to parse client certificate: %w", err)
	}

	return &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
		ServerName:   serverName,
	}, nil
}
