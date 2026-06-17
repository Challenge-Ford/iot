package main

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
)

var dtcCodes = []string{
	"P0128", "P0171", "P0174", "P0300", "P0301", "P0302", "P0303", "P0304",
	"P0401", "P0420", "P0442", "P0507",
}

func runTUI() error {
	fields := map[string]*string{
		"lat":             strPtr(""),
		"lng":             strPtr(""),
		"gps_speed":       strPtr(""),
		"heading":         strPtr(""),
		"hdop":            strPtr(""),
		"speed":           strPtr(""),
		"rpm":             strPtr(""),
		"coolant_temp":    strPtr(""),
		"engine_load":     strPtr(""),
		"throttle_pos":    strPtr(""),
		"fuel_level":      strPtr(""),
		"battery_voltage": strPtr(""),
		"dtcs":            strPtr(""),
	}
	var action string
	var diagnosticsMode string

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
			huh.NewSelect[string]().
				Title("Diagnostics").
				Options(
					huh.NewOption("Not observed", "not_observed"),
					huh.NewOption("No open DTCs", "none"),
					huh.NewOption("Open DTCs", "open"),
				).
				Value(&diagnosticsMode),
		),
	).Run()
	if err != nil {
		return err
	}

	if action == "random_send" || action == "random_edit" {
		*fields["lat"] = fmt.Sprintf("%.6f", -23.55+rand.Float64()*0.1-0.05)
		*fields["lng"] = fmt.Sprintf("%.6f", -46.63+rand.Float64()*0.1-0.05)
		*fields["gps_speed"] = strconv.Itoa(rand.IntN(121))
		*fields["heading"] = fmt.Sprintf("%.1f", rand.Float64()*360)
		*fields["hdop"] = fmt.Sprintf("%.1f", 0.6+rand.Float64()*1.8)
		*fields["speed"] = strconv.Itoa(rand.IntN(121))
		*fields["rpm"] = strconv.Itoa(800 + rand.IntN(5201))
		*fields["coolant_temp"] = fmt.Sprintf("%.1f", 70+rand.Float64()*30)
		*fields["engine_load"] = fmt.Sprintf("%.1f", rand.Float64()*100)
		*fields["throttle_pos"] = fmt.Sprintf("%.1f", rand.Float64()*100)
		*fields["fuel_level"] = fmt.Sprintf("%.1f", 10+rand.Float64()*90)
		*fields["battery_voltage"] = fmt.Sprintf("%.2f", 11.5+rand.Float64()*3)
		if diagnosticsMode == "open" {
			*fields["dtcs"] = dtcCodes[rand.IntN(len(dtcCodes))]
		}
	}

	if action != "random_send" {
		inputs := []huh.Field{
			huh.NewInput().Title("Latitude").Value(fields["lat"]),
			huh.NewInput().Title("Longitude").Value(fields["lng"]),
			huh.NewInput().Title("GPS speed (km/h)").Value(fields["gps_speed"]),
			huh.NewInput().Title("Heading").Value(fields["heading"]),
			huh.NewInput().Title("HDOP").Value(fields["hdop"]),
			huh.NewInput().Title("OBD speed (km/h)").Value(fields["speed"]),
			huh.NewInput().Title("RPM").Value(fields["rpm"]),
			huh.NewInput().Title("Coolant temp (C)").Value(fields["coolant_temp"]),
			huh.NewInput().Title("Engine load (%)").Value(fields["engine_load"]),
			huh.NewInput().Title("Throttle position (%)").Value(fields["throttle_pos"]),
			huh.NewInput().Title("Fuel level (%)").Value(fields["fuel_level"]),
			huh.NewInput().Title("Battery voltage (V)").Value(fields["battery_voltage"]),
		}
		if diagnosticsMode == "open" {
			inputs = append(inputs, huh.NewInput().Title("Open DTCs").Value(fields["dtcs"]))
		}
		err = huh.NewForm(huh.NewGroup(inputs...)).Run()
		if err != nil {
			return err
		}
	}

	state := map[string]any{}
	position := map[string]any{}
	powertrain := map[string]any{}
	fuel := map[string]any{}
	electrical := map[string]any{}

	parseFloat(position, "lat", *fields["lat"])
	parseFloat(position, "lng", *fields["lng"])
	parseFloat(position, "gps_speed", *fields["gps_speed"])
	parseFloat(position, "heading", *fields["heading"])
	parseFloat(position, "hdop", *fields["hdop"])
	setBlock(state, "position", position)

	parseInt(powertrain, "speed", *fields["speed"])
	parseInt(powertrain, "rpm", *fields["rpm"])
	parseFloat(powertrain, "coolant_temp", *fields["coolant_temp"])
	parseFloat(powertrain, "engine_load", *fields["engine_load"])
	parseFloat(powertrain, "throttle_pos", *fields["throttle_pos"])
	setBlock(state, "powertrain", powertrain)

	parseFloat(fuel, "fuel_level", *fields["fuel_level"])
	setBlock(state, "fuel", fuel)

	parseFloat(electrical, "battery_voltage", *fields["battery_voltage"])
	setBlock(state, "electrical", electrical)

	switch diagnosticsMode {
	case "none":
		state["diagnostics"] = map[string]any{"open_dtcs": []string{}}
	case "open":
		state["diagnostics"] = map[string]any{"open_dtcs": splitCSV(*fields["dtcs"])}
	}

	if len(state) == 0 {
		return fmt.Errorf("at least one state field must be provided")
	}

	return publishState(map[string]any{
		"observed_at": time.Now().UTC().Format(time.RFC3339Nano),
		"state":       state,
		"observation": map[string]any{"errors": []string{}},
	})
}

func parseFloat(m map[string]any, key, val string) {
	if val == "" {
		return
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err == nil {
		m[key] = f
	}
}

func parseInt(m map[string]any, key, val string) {
	if val == "" {
		return
	}
	i, err := strconv.Atoi(strings.TrimSpace(val))
	if err == nil {
		m[key] = i
	}
}

func strPtr(s string) *string { return &s }
