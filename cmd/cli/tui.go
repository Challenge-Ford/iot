package main

import (
	"fmt"
	"math/rand/v2"
	"strconv"

	"github.com/charmbracelet/huh"
)

var dtcCodes = []string{
	"P0128", "P0171", "P0174", "P0300", "P0301", "P0302", "P0303", "P0304",
	"P0401", "P0420", "P0442", "P0507",
}

func runTUI() error {
	var msgType string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Message type").
				Options(
					huh.NewOption("Telemetry", "telemetry"),
					huh.NewOption("DTC", "dtc"),
				).
				Value(&msgType),
		),
	).Run()
	if err != nil {
		return err
	}

	switch msgType {
	case "telemetry":
		return tuiTelemetry()
	case "dtc":
		return tuiDTC()
	}
	return nil
}

func tuiTelemetry() error {
	fields := map[string]*string{
		"lat":             strPtr(""),
		"lng":             strPtr(""),
		"speed":           strPtr(""),
		"rpm":             strPtr(""),
		"coolant_temp":    strPtr(""),
		"engine_load":     strPtr(""),
		"throttle_pos":    strPtr(""),
		"fuel_level":      strPtr(""),
		"battery_voltage": strPtr(""),
	}

	var action string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How do you want to fill the values?").
				Options(
					huh.NewOption("Randomize and send", "random_send"),
					huh.NewOption("Randomize and edit", "random_edit"),
					huh.NewOption("Fill manually", "manual"),
				).
				Value(&action),
		),
	).Run()
	if err != nil {
		return err
	}

	if action == "random_send" || action == "random_edit" {
		*fields["lat"] = fmt.Sprintf("%.6f", -23.55+rand.Float64()*0.1-0.05)
		*fields["lng"] = fmt.Sprintf("%.6f", -46.63+rand.Float64()*0.1-0.05)
		*fields["speed"] = strconv.Itoa(rand.IntN(121))
		*fields["rpm"] = strconv.Itoa(800 + rand.IntN(5201))
		*fields["coolant_temp"] = fmt.Sprintf("%.1f", 70+rand.Float64()*30)
		*fields["engine_load"] = fmt.Sprintf("%.1f", rand.Float64()*100)
		*fields["throttle_pos"] = fmt.Sprintf("%.1f", rand.Float64()*100)
		*fields["fuel_level"] = fmt.Sprintf("%.1f", 10+rand.Float64()*90)
		*fields["battery_voltage"] = fmt.Sprintf("%.2f", 11.5+rand.Float64()*3)
	}

	if action != "random_send" {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("Latitude").Value(fields["lat"]),
				huh.NewInput().Title("Longitude").Value(fields["lng"]),
				huh.NewInput().Title("Speed (km/h)").Value(fields["speed"]),
				huh.NewInput().Title("RPM").Value(fields["rpm"]),
			),
			huh.NewGroup(
				huh.NewInput().Title("Coolant temp (°C)").Value(fields["coolant_temp"]),
				huh.NewInput().Title("Engine load (%)").Value(fields["engine_load"]),
				huh.NewInput().Title("Throttle position (%)").Value(fields["throttle_pos"]),
				huh.NewInput().Title("Fuel level (%)").Value(fields["fuel_level"]),
				huh.NewInput().Title("Battery voltage (V)").Value(fields["battery_voltage"]),
			),
		).Run()
		if err != nil {
			return err
		}
	}

	payload := map[string]any{}
	parseFloat(payload, "lat", *fields["lat"])
	parseFloat(payload, "lng", *fields["lng"])
	parseInt(payload, "speed", *fields["speed"])
	parseInt(payload, "rpm", *fields["rpm"])
	parseFloat(payload, "coolant_temp", *fields["coolant_temp"])
	parseFloat(payload, "engine_load", *fields["engine_load"])
	parseFloat(payload, "throttle_pos", *fields["throttle_pos"])
	parseFloat(payload, "fuel_level", *fields["fuel_level"])
	parseFloat(payload, "battery_voltage", *fields["battery_voltage"])

	if len(payload) == 0 {
		return fmt.Errorf("at least one field must be provided")
	}

	return publishMsg("telemetry", payload)
}

func tuiDTC() error {
	var action string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("How do you want to fill the values?").
				Options(
					huh.NewOption("Randomize and send", "random_send"),
					huh.NewOption("Randomize and edit", "random_edit"),
					huh.NewOption("Fill manually", "manual"),
				).
				Value(&action),
		),
	).Run()
	if err != nil {
		return err
	}

	var code, status string

	if action == "random_send" || action == "random_edit" {
		code = dtcCodes[rand.IntN(len(dtcCodes))]
		// opened weighted 70%
		if rand.IntN(10) < 7 {
			status = "opened"
		} else {
			status = "closed"
		}
	}

	if action != "random_send" {
		err = huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("DTC code (e.g. P0300)").
					Value(&code),
				huh.NewSelect[string]().
					Title("Status").
					Options(
						huh.NewOption("Opened", "opened"),
						huh.NewOption("Closed", "closed"),
					).
					Value(&status),
			),
		).Run()
		if err != nil {
			return err
		}
	}

	if code == "" {
		return fmt.Errorf("code is required")
	}

	return publishMsg("dtc", map[string]any{"code": code, "status": status})
}


func parseFloat(m map[string]any, key, val string) {
	if val == "" {
		return
	}
	f, err := strconv.ParseFloat(val, 64)
	if err == nil {
		m[key] = f
	}
}

func parseInt(m map[string]any, key, val string) {
	if val == "" {
		return
	}
	i, err := strconv.Atoi(val)
	if err == nil {
		m[key] = i
	}
}

func strPtr(s string) *string { return &s }
