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
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"torque-iot/internal/core/logger"
)

const (
	serviceClientID = "torque-listener"

	queueTelemetry = "torque.telemetry"
	queueDTC       = "torque.dtc"
)

var topicQueues = map[string]string{
	"telemetry": queueTelemetry,
	"dtc":       queueDTC,
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

	// --- TLS ---
	tlsConfig, err := buildTLSConfig(
		mustEnv("MQTT_CA_CERT"),
		mustEnv("MQTT_CERT"),
		mustEnv("MQTT_KEY"),
		os.Getenv("MQTT_SERVER_NAME"),
	)
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

	for _, queue := range []string{queueTelemetry, queueDTC} {
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
		var mu sync.Mutex
		for suffix := range topicQueues {
			topic := "torque/vehicles/+/" + suffix
			token := c.Subscribe(topic, 1, makeHandler(log, ch, &mu, suffix))
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

func makeHandler(log *zap.Logger, ch *amqp.Channel, mu *sync.Mutex, suffix string) mqtt.MessageHandler {
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

			mu.Lock()
			err = ch.PublishWithContext(context.Background(), "", queue, false, false, amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				Timestamp:    time.Now(),
				Body:         body,
			})
			mu.Unlock()
			if err != nil {
				log.Error("failed to publish to rabbitmq", zap.String("queue", queue), zap.Error(err))
			}
		}

		log.Debug("forwarded messages",
			zap.String("topic", msg.Topic()),
			zap.String("queue", queue),
			zap.Int("count", len(items)),
		)
	}
}

func vinFromTopic(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) != 4 {
		return ""
	}
	return parts[2]
}

func unpackPayload(payload []byte) ([]map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(payload, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return nil, err
	}
	return []map[string]any{obj}, nil
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
