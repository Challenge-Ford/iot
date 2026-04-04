package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/cobra"
)

var (
	broker     string
	serverName string
	caFile     string
	certFile   string
	keyFile    string
	vin        string
)

func main() {
	root := &cobra.Command{
		Use:   "cli",
		Short: "Torque device simulator",
	}

	root.PersistentFlags().StringVar(&broker, "broker", "ssl://localhost:8884", "MQTT broker URL")
	root.PersistentFlags().StringVar(&serverName, "server-name", "emqx", "TLS server name")
	root.PersistentFlags().StringVar(&caFile, "ca-cert", "certs/device/ca.crt", "CA certificate path")
	root.PersistentFlags().StringVar(&certFile, "cert", "certs/device/device.crt", "Client certificate path")
	root.PersistentFlags().StringVar(&keyFile, "key", "certs/device/device.key", "Client private key path")
	root.PersistentFlags().StringVar(&vin, "vin", "", "Vehicle VIN (default: read from certs/device/meta.json)")

	publish := &cobra.Command{
		Use:   "publish",
		Short: "Publish a message to the broker",
	}
	publish.AddCommand(telemetryCmd(), dtcCmd(), sessionCmd())
	root.AddCommand(publish)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func telemetryCmd() *cobra.Command {
	var (
		lat, lng, alt, gpsSpeed, heading, hdop                    float64
		coolantTemp, intakeTemp, engineLoad, throttlePos           float64
		fuelLevel, fuelTrimShort, fuelTrimLong, maf, batteryVoltage float64
		rpm, speed                                                 int
	)

	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Publish telemetry data",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			setFloat(cmd, payload, "lat", lat)
			setFloat(cmd, payload, "lng", lng)
			setFloat(cmd, payload, "alt", alt)
			setFloat(cmd, payload, "gps_speed", gpsSpeed)
			setFloat(cmd, payload, "heading", heading)
			setFloat(cmd, payload, "hdop", hdop)
			setInt(cmd, payload, "rpm", rpm)
			setInt(cmd, payload, "speed", speed)
			setFloat(cmd, payload, "coolant_temp", coolantTemp)
			setFloat(cmd, payload, "intake_temp", intakeTemp)
			setFloat(cmd, payload, "engine_load", engineLoad)
			setFloat(cmd, payload, "throttle_pos", throttlePos)
			setFloat(cmd, payload, "fuel_level", fuelLevel)
			setFloat(cmd, payload, "fuel_trim_short", fuelTrimShort)
			setFloat(cmd, payload, "fuel_trim_long", fuelTrimLong)
			setFloat(cmd, payload, "maf", maf)
			setFloat(cmd, payload, "battery_voltage", batteryVoltage)
			if len(payload) == 0 {
				return fmt.Errorf("at least one field must be provided")
			}
			return publishMsg("telemetry", payload)
		},
	}

	cmd.Flags().Float64Var(&lat, "lat", 0, "Latitude")
	cmd.Flags().Float64Var(&lng, "lng", 0, "Longitude")
	cmd.Flags().Float64Var(&alt, "alt", 0, "Altitude (m)")
	cmd.Flags().Float64Var(&gpsSpeed, "gps-speed", 0, "GPS speed (km/h)")
	cmd.Flags().Float64Var(&heading, "heading", 0, "Heading (degrees)")
	cmd.Flags().Float64Var(&hdop, "hdop", 0, "HDOP")
	cmd.Flags().IntVar(&rpm, "rpm", 0, "Engine RPM")
	cmd.Flags().IntVar(&speed, "speed", 0, "OBD speed (km/h)")
	cmd.Flags().Float64Var(&coolantTemp, "coolant-temp", 0, "Coolant temperature (°C)")
	cmd.Flags().Float64Var(&intakeTemp, "intake-temp", 0, "Intake air temperature (°C)")
	cmd.Flags().Float64Var(&engineLoad, "engine-load", 0, "Engine load (%)")
	cmd.Flags().Float64Var(&throttlePos, "throttle-pos", 0, "Throttle position (%)")
	cmd.Flags().Float64Var(&fuelLevel, "fuel-level", 0, "Fuel level (%)")
	cmd.Flags().Float64Var(&fuelTrimShort, "fuel-trim-short", 0, "Short-term fuel trim (%)")
	cmd.Flags().Float64Var(&fuelTrimLong, "fuel-trim-long", 0, "Long-term fuel trim (%)")
	cmd.Flags().Float64Var(&maf, "maf", 0, "Mass air flow (g/s)")
	cmd.Flags().Float64Var(&batteryVoltage, "battery-voltage", 0, "Battery voltage (V)")

	return cmd
}

func dtcCmd() *cobra.Command {
	var code, status string

	cmd := &cobra.Command{
		Use:   "dtc",
		Short: "Publish a DTC event",
		RunE: func(cmd *cobra.Command, args []string) error {
			if code == "" {
				return fmt.Errorf("--code is required")
			}
			if status != "opened" && status != "closed" {
				return fmt.Errorf("--status must be 'opened' or 'closed'")
			}
			return publishMsg("dtc", map[string]any{
				"code":   code,
				"status": status,
			})
		},
	}

	cmd.Flags().StringVar(&code, "code", "", "DTC code (e.g. P0300)")
	cmd.Flags().StringVar(&status, "status", "", "Event status: opened or closed")

	return cmd
}

func sessionCmd() *cobra.Command {
	var event string

	cmd := &cobra.Command{
		Use:   "session",
		Short: "Publish a session event",
		RunE: func(cmd *cobra.Command, args []string) error {
			if event != "start" && event != "end" {
				return fmt.Errorf("--event must be 'start' or 'end'")
			}
			return publishMsg("session", map[string]any{"event": event})
		},
	}

	cmd.Flags().StringVar(&event, "event", "", "Session event: start or end")

	return cmd
}

func publishMsg(topic string, payload map[string]any) error {
	v, err := resolveVIN()
	if err != nil {
		return err
	}

	tlsCfg, err := buildTLSConfig()
	if err != nil {
		return err
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID("torque-cli")
	opts.SetTLSConfig(tlsCfg)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("connect: %w", token.Error())
	}
	defer client.Disconnect(250)

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	fullTopic := fmt.Sprintf("torque/vehicles/%s/%s", v, topic)
	token = client.Publish(fullTopic, 1, false, body)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish: %w", token.Error())
	}

	fmt.Printf("published to %s: %s\n", fullTopic, body)
	return nil
}

func resolveVIN() (string, error) {
	if vin != "" {
		return vin, nil
	}
	data, err := os.ReadFile("certs/device/meta.json")
	if err != nil {
		return "", fmt.Errorf("could not read certs/device/meta.json (use --vin to specify): %w", err)
	}
	var meta struct {
		VIN string `json:"vin"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}
	if meta.VIN == "" {
		return "", fmt.Errorf("vin not found in meta.json")
	}
	return meta.VIN, nil
}

func buildTLSConfig() (*tls.Config, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse ca certificate")
	}
	clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
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

func setFloat(cmd *cobra.Command, m map[string]any, name string, val float64) {
	if cmd.Flags().Changed(name) {
		m[name] = val
	}
}

func setInt(cmd *cobra.Command, m map[string]any, name string, val int) {
	if cmd.Flags().Changed(name) {
		m[name] = val
	}
}
