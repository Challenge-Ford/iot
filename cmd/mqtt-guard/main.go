package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
	"torque-iot/cmd/mqtt-guard/handler"
	guardmw "torque-iot/cmd/mqtt-guard/middleware"
	"torque-iot/internal/core/logger"
)

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
		log.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("failed to ping database", zap.Error(err))
	}

	var serviceCNs []string
	if raw := os.Getenv("MQTT_SERVICE_CNS"); raw != "" {
		serviceCNs = strings.Split(raw, ",")
	}

	guard := handler.NewGuard(db, log, serviceCNs)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(guardmw.ErrorLogger(log))
	r.Use(guardmw.SharedSecret(mustEnv("MQTT_GUARD_SECRET")))

	r.Post("/mqtt/auth", guard.Auth)
	r.Post("/mqtt/acl", guard.ACL)

	port := mustEnv("PORT")
	log.Info("starting mqtt-guard", zap.String("port", port))
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal("failed to serve", zap.Error(err))
	}
}
