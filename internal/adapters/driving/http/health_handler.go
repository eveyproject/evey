package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

type healthResponse struct {
	Status        string `json:"status"`
	DB            string `json:"db"`
	Meilisearch   string `json:"meilisearch"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func (s *HTTPServer) healthzHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.health.Check")
	defer span.End()
	r = r.WithContext(ctx)
	dbStatus := "ok"
	if s.DB != nil {
		if err := s.DB.Ping(r.Context()); err != nil {
			dbStatus = "error"
		}
	}

	meiliStatus := "not_configured"
	if s.SearchHealth != nil {
		if err := s.SearchHealth.HealthCheck(r.Context()); err != nil {
			meiliStatus = "degraded"
		} else {
			meiliStatus = "ok"
		}
	}

	version := s.Version
	if version == "" {
		version = os.Getenv("EVEY_VERSION")
		if version == "" {
			version = "dev"
		}
	}

	overallStatus := "ok"
	if dbStatus == "error" {
		overallStatus = "error"
	} else if meiliStatus == "degraded" {
		overallStatus = "degraded"
	}

	resp := healthResponse{
		Status:        overallStatus,
		DB:            dbStatus,
		Meilisearch:   meiliStatus,
		Version:       version,
		UptimeSeconds: int64(time.Since(s.StartTime).Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	if dbStatus == "error" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}
