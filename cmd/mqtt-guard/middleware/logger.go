package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

func ErrorLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rw, r)
			if rw.status >= 400 {
				log.Warn("request error",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", rw.status),
					zap.Duration("duration", time.Since(start)),
				)
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}
