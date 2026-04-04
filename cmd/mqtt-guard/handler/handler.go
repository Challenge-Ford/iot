package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type result struct {
	Result string `json:"result"`
}

var allow = result{"allow"}
var deny = result{"deny"}

type authInput struct {
	ClientID string `json:"clientid"`
	CN       string `json:"cn"`
}

type aclInput struct {
	ClientID string `json:"clientid"`
	CN       string `json:"cn"`
	Topic    string `json:"topic"`
	Action   string `json:"action"`
}

type Guard struct {
	db         *sql.DB
	serviceCNs map[string]struct{}
}

func NewGuard(db *sql.DB, serviceCNs []string) *Guard {
	m := make(map[string]struct{}, len(serviceCNs))
	for _, cn := range serviceCNs {
		m[cn] = struct{}{}
	}
	return &Guard{db: db, serviceCNs: m}
}

func (g *Guard) Auth(w http.ResponseWriter, r *http.Request) {
	var input authInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, deny)
		return
	}

	identity := identity(input.CN, input.ClientID)
	if identity == "" {
		writeJSON(w, deny)
		return
	}

	if _, ok := g.serviceCNs[identity]; ok {
		writeJSON(w, allow)
		return
	}

	var exists bool
	err := g.db.QueryRowContext(r.Context(),
		`SELECT EXISTS(
			SELECT 1 FROM device.devices
			WHERE certificate_cn = $1
			  AND vehicle_id IS NOT NULL
			  AND deleted_at IS NULL
		)`, identity).Scan(&exists)
	if err != nil || !exists {
		writeJSON(w, deny)
		return
	}

	writeJSON(w, allow)
}

func (g *Guard) ACL(w http.ResponseWriter, r *http.Request) {
	var input aclInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, deny)
		return
	}

	identity := identity(input.CN, input.ClientID)
	if identity == "" || input.Topic == "" {
		writeJSON(w, deny)
		return
	}

	if _, ok := g.serviceCNs[identity]; ok {
		if input.Action == "subscribe" && isServiceTopic(input.Topic) {
			writeJSON(w, allow)
			return
		}
		writeJSON(w, deny)
		return
	}

	if input.Action != "publish" {
		writeJSON(w, deny)
		return
	}

	var vin string
	err := g.db.QueryRowContext(r.Context(),
		`SELECT vehicle_vin FROM device.devices
		 WHERE certificate_cn = $1
		   AND vehicle_id IS NOT NULL
		   AND vehicle_vin IS NOT NULL
		   AND deleted_at IS NULL`,
		identity).Scan(&vin)
	if err != nil {
		writeJSON(w, deny)
		return
	}

	for _, suffix := range []string{"telemetry", "dtc", "session"} {
		if input.Topic == fmt.Sprintf("torque/vehicles/%s/%s", vin, suffix) {
			writeJSON(w, allow)
			return
		}
	}

	writeJSON(w, deny)
}

func identity(cn, clientID string) string {
	if cn != "" {
		return cn
	}
	return clientID
}

func isServiceTopic(topic string) bool {
	for _, suffix := range []string{"telemetry", "dtc", "session"} {
		if topic == "torque/vehicles/+/"+suffix {
			return true
		}
		if strings.HasPrefix(topic, "torque/vehicles/") && strings.HasSuffix(topic, "/"+suffix) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v)
}
