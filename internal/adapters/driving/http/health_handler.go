package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"time"
)

type healthResponse struct {
	Status        string `json:"status"`
	DB            string `json:"db"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

func (s *HTTPServer) healthzHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if s.DB != nil {
		if err := s.DB.Ping(r.Context()); err != nil {
			dbStatus = "error"
		}
	}

	version := s.Version
	if version == "" {
		version = os.Getenv("EVEY_VERSION")
		if version == "" {
			version = "dev"
		}
	}

	resp := healthResponse{
		Status:        "ok",
		DB:            dbStatus,
		Version:       version,
		UptimeSeconds: int64(time.Since(s.StartTime).Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}
