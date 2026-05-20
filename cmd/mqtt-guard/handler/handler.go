package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

type result struct {
	Result string `json:"result"`
}

var allow = result{"allow"}
var deny = result{"deny"}

type authInput struct {
	CN string `json:"cn"`
}

type aclInput struct {
	CN     string `json:"cn"`
	Topic  string `json:"topic"`
	Action string `json:"action"`
}

type Guard struct {
	db         *sql.DB
	log        *zap.Logger
	serviceCNs map[string]struct{}
}

func NewGuard(db *sql.DB, log *zap.Logger, serviceCNs []string) *Guard {
	m := make(map[string]struct{}, len(serviceCNs))
	for _, cn := range serviceCNs {
		m[cn] = struct{}{}
	}
	return &Guard{db: db, log: log, serviceCNs: m}
}

func (g *Guard) Auth(w http.ResponseWriter, r *http.Request) {
	var input authInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, deny)
		return
	}

	identity := input.CN
	if identity == "" {
		writeJSON(w, deny)
		return
	}

	if _, ok := g.serviceCNs[identity]; ok {
		g.log.Info("auth allow (service)", zap.String("cn", identity))
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
		g.log.Warn("auth deny", zap.String("cn", identity))
		writeJSON(w, deny)
		return
	}

	g.log.Info("auth allow (device)", zap.String("cn", identity))
	writeJSON(w, allow)
}

func (g *Guard) ACL(w http.ResponseWriter, r *http.Request) {
	var input aclInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, deny)
		return
	}

	identity := input.CN
	if identity == "" || input.Topic == "" {
		writeJSON(w, deny)
		return
	}

	if _, ok := g.serviceCNs[identity]; ok {
		if input.Action == "subscribe" && isServiceTopic(input.Topic) {
			g.log.Info("acl allow (service)", zap.String("cn", identity), zap.String("topic", input.Topic), zap.String("action", input.Action))
			writeJSON(w, allow)
			return
		}
		g.log.Info("acl deny (service)", zap.String("cn", identity), zap.String("topic", input.Topic), zap.String("action", input.Action))
		writeJSON(w, deny)
		return
	}

	if input.Action != "publish" {
		g.log.Info("acl deny (device not publish)", zap.String("cn", identity), zap.String("topic", input.Topic), zap.String("action", input.Action))
		writeJSON(w, deny)
		return
	}

	var vin string
	err := g.db.QueryRowContext(r.Context(),
		`SELECT v.vin
		 FROM device.devices d
		 JOIN vehicle.vehicles v ON v.id = d.vehicle_id AND v.deleted_at IS NULL
		 WHERE d.certificate_cn = $1
		   AND d.vehicle_id IS NOT NULL
		   AND d.deleted_at IS NULL`,
		identity).Scan(&vin)
	if err != nil {
		g.log.Info("acl deny (device not found)", zap.String("cn", identity), zap.String("topic", input.Topic))
		writeJSON(w, deny)
		return
	}

	for _, suffix := range []string{"telemetry", "dtc", "session"} {
		if input.Topic == fmt.Sprintf("torque/vehicles/%s/%s", vin, suffix) {
			g.log.Info("acl allow (device)", zap.String("cn", identity), zap.String("topic", input.Topic))
			writeJSON(w, allow)
			return
		}
	}

	g.log.Info("acl deny (device wrong topic)", zap.String("cn", identity), zap.String("topic", input.Topic))
	writeJSON(w, deny)
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
