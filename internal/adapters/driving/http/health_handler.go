package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

// unknownBuildStamp mirrors cmd/evey's unknownStamp: the value reported when a
// build cannot name itself. Deployment tooling treats it as a failure, so it
// must never be substituted by "" or by a guess.
const unknownBuildStamp = "unknown"

type healthResponse struct {
	Status      string `json:"status"`
	DB          string `json:"db"`
	Meilisearch string `json:"meilisearch"`
	Version     string `json:"version"`
	// Commit is the git SHA the binary was built from and BuildDate its RFC3339
	// build timestamp - the pair that answers "is production running my last
	// commit?" without a shell on the host. `version` alone is a branch or tag
	// name and cannot: two images months apart both report "beta-0.0.1-full".
	Commit        string `json:"commit"`
	BuildDate     string `json:"build_date"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	// Name is the admin-chosen instance name (Server settings). /healthz is
	// the mobile/TV discovery probe, so the name reaches the server list from
	// the same answer that proves this is an Evey server.
	Name string `json:"name,omitempty"`
	// Clients names the app builds released with this server, so an app can
	// tell it is behind: the Play Store auto-update rarely runs on a TV.
	Clients *healthClients `json:"clients,omitempty"`
	// Demo says this is the public demo instance: mobile and TV read it from
	// the same discovery answer and show the banner before login.
	Demo bool `json:"demo,omitempty"`
	// DemoExtended says this is the private extended demo. Demo is false
	// there: the clients' read-only locks and credentials hint belong to the
	// public demo only; this flag only tells the clients to keep their hands off the network settings.
	DemoExtended bool `json:"demo_extended,omitempty"`
}

type healthClients struct {
	TV *ClientRelease `json:"tv,omitempty"`
}

// ClientRelease is a client app build, read from the app's app.json when the
// server was built. The zero value means the build was not stamped.
type ClientRelease struct {
	Version     string `json:"version"`
	VersionCode int    `json:"version_code"`
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
		Commit:        buildStampOrUnknown(s.Commit),
		BuildDate:     buildStampOrUnknown(s.BuildDate),
		UptimeSeconds: int64(time.Since(s.StartTime).Seconds()),
	}
	if s.Cfg != nil {
		resp.Name = s.Cfg.EffectiveServerName()
		resp.Demo = s.Cfg.Server.Demo && !s.Cfg.Server.DemoExtended
		resp.DemoExtended = s.Cfg.Server.DemoExtended
	}
	if s.TVRelease.VersionCode > 0 {
		tv := s.TVRelease
		resp.Clients = &healthClients{TV: &tv}
	}

	w.Header().Set("Content-Type", "application/json")
	if dbStatus == "error" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}

// buildStampOrUnknown keeps a blank stamp off the wire: "" is
// indistinguishable from an older server that never had the field, and the
// caller would have to guess. unknownBuildStamp is an explicit answer, and
// scripts/verify-deploy.sh fails on it instead of assuming a match.
func buildStampOrUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return unknownBuildStamp
	}
	return s
}
