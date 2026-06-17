package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/cobra"
)

var (
	broker     string
	serverName string
	caFile     string
	certFile   string
	keyFile    string
	vehicleID  string
)

func main() {
	root := &cobra.Command{
		Use:   "cli",
		Short: "Torque device simulator",
	}

	root.PersistentFlags().StringVar(&broker, "broker", "ssl://localhost:8883", "MQTT broker URL")
	root.PersistentFlags().StringVar(&serverName, "server-name", "emqx", "TLS server name")
	root.PersistentFlags().StringVar(&caFile, "ca-cert", "certs/device/ca.crt", "CA certificate path")
	root.PersistentFlags().StringVar(&certFile, "cert", "certs/device/device.crt", "Client certificate path")
	root.PersistentFlags().StringVar(&keyFile, "key", "certs/device/device.key", "Client private key path")
	root.PersistentFlags().StringVar(&vehicleID, "vehicle-id", "", "Vehicle UUID (default: read from certs/device/meta.json)")

	publish := &cobra.Command{
		Use:   "publish",
		Short: "Publish a message to the broker",
	}
	publish.AddCommand(stateCmd())

	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "Interactive TUI for publishing messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI()
		},
	}

	root.AddCommand(publish, tuiCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func stateCmd() *cobra.Command {
	var (
		lat, lng, alt, gpsSpeed, heading, hdop                      float64
		coolantTemp, intakeTemp, engineLoad, throttlePos            float64
		fuelLevel, fuelTrimShort, fuelTrimLong, maf, batteryVoltage float64
		rpm, speed                                                  int
		dtcs                                                        string
		noDtcs                                                      bool
	)

	cmd := &cobra.Command{
		Use:   "state",
		Short: "Publish a vehicle state snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			state := map[string]any{}
			position := map[string]any{}
			powertrain := map[string]any{}
			fuel := map[string]any{}
			electrical := map[string]any{}

			setFloat(cmd, position, "lat", lat)
			setFloat(cmd, position, "lng", lng)
			setFloat(cmd, position, "alt", alt)
			setFloat(cmd, position, "gps_speed", gpsSpeed)
			setFloat(cmd, position, "heading", heading)
			setFloat(cmd, position, "hdop", hdop)
			setBlock(state, "position", position)

			setInt(cmd, powertrain, "rpm", rpm)
			setInt(cmd, powertrain, "speed", speed)
			setFloat(cmd, powertrain, "coolant_temp", coolantTemp)
			setFloat(cmd, powertrain, "intake_temp", intakeTemp)
			setFloat(cmd, powertrain, "engine_load", engineLoad)
			setFloat(cmd, powertrain, "throttle_pos", throttlePos)
			setFloat(cmd, powertrain, "maf", maf)
			setBlock(state, "powertrain", powertrain)

			setFloat(cmd, fuel, "fuel_level", fuelLevel)
			setFloat(cmd, fuel, "fuel_trim_short", fuelTrimShort)
			setFloat(cmd, fuel, "fuel_trim_long", fuelTrimLong)
			setBlock(state, "fuel", fuel)

			setFloat(cmd, electrical, "battery_voltage", batteryVoltage)
			setBlock(state, "electrical", electrical)

			if cmd.Flags().Changed("dtcs") || noDtcs {
				openDTCs := splitCSV(dtcs)
				if noDtcs {
					openDTCs = []string{}
				}
				state["diagnostics"] = map[string]any{"open_dtcs": openDTCs}
			}

			if len(state) == 0 {
				return fmt.Errorf("at least one state field must be provided")
			}

			return publishState(map[string]any{
				"observed_at": time.Now().UTC().Format(time.RFC3339Nano),
				"state":       state,
				"observation": map[string]any{"errors": []string{}},
			})
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
	cmd.Flags().Float64Var(&coolantTemp, "coolant-temp", 0, "Coolant temperature (C)")
	cmd.Flags().Float64Var(&intakeTemp, "intake-temp", 0, "Intake air temperature (C)")
	cmd.Flags().Float64Var(&engineLoad, "engine-load", 0, "Engine load (%)")
	cmd.Flags().Float64Var(&throttlePos, "throttle-pos", 0, "Throttle position (%)")
	cmd.Flags().Float64Var(&fuelLevel, "fuel-level", 0, "Fuel level (%)")
	cmd.Flags().Float64Var(&fuelTrimShort, "fuel-trim-short", 0, "Short-term fuel trim (%)")
	cmd.Flags().Float64Var(&fuelTrimLong, "fuel-trim-long", 0, "Long-term fuel trim (%)")
	cmd.Flags().Float64Var(&maf, "maf", 0, "Mass air flow (g/s)")
	cmd.Flags().Float64Var(&batteryVoltage, "battery-voltage", 0, "Battery voltage (V)")
	cmd.Flags().StringVar(&dtcs, "dtcs", "", "Comma-separated open DTC codes")
	cmd.Flags().BoolVar(&noDtcs, "no-dtcs", false, "Send diagnostics as observed with no open DTCs")

	return cmd
}

func publishState(payload map[string]any) error {
	id, err := resolveVehicleID()
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

	topic := fmt.Sprintf("torque/vehicles/%s/state", id)
	token = client.Publish(topic, 1, false, body)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("publish: %w", token.Error())
	}

	fmt.Printf("published to %s: %s\n", topic, body)
	return nil
}

func resolveVehicleID() (string, error) {
	if vehicleID != "" {
		return vehicleID, nil
	}
	data, err := os.ReadFile("certs/device/meta.json")
	if err != nil {
		return "", fmt.Errorf("could not read certs/device/meta.json (use --vehicle-id to specify): %w", err)
	}
	var meta struct {
		VehicleID string `json:"vehicle_id"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", err
	}
	if meta.VehicleID == "" {
		return "", fmt.Errorf("vehicle_id not found in meta.json")
	}
	return meta.VehicleID, nil
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

func setBlock(state map[string]any, name string, block map[string]any) {
	if len(block) > 0 {
		state[name] = block
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		code := strings.TrimSpace(part)
		if code != "" {
			out = append(out, code)
		}
	}
	return out
}
