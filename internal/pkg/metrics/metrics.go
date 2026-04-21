package metrics

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HLSSessionsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_active_hls_sessions",
		Help: "Current number of active HLS transcode sessions.",
	})

	TranscodesActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_transcodes_active",
		Help: "Current number of active background transcode jobs.",
	})

	HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "evey_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	DBConnectionsOpen = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_db_connections_open",
		Help: "Number of open DB connections.",
	})

	DBConnectionsIdle = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_db_connections_idle",
		Help: "Number of idle DB connections.",
	})

	DBConnectionsInUse = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_db_connections_in_use",
		Help: "Number of DB connections currently in use.",
	})

	MediaItemsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "evey_media_items_total",
		Help: "Total number of media items by type.",
	}, []string{"type"})

	// AuthLatency tracks login/refresh/register latency. Separated from the
	// generic HTTP histogram so the auth SLO (p99 < 500ms) can be alerted on
	// without noise from slow transcoding or scan endpoints.
	AuthLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "evey_auth_latency_seconds",
		Help:    "Auth operation latency in seconds.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"operation", "result"})

	// ScanQueueDepth reports the number of files waiting to be scanned
	// (post-diff, pre-process). Actionable for alerting "scan backlog growing".
	ScanQueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "evey_scan_queue_depth",
		Help: "Number of files enqueued for library scan processing.",
	}, []string{"library_id"})

	// TranscodeQueueDepth counts transcode requests blocked waiting for
	// a semaphore slot (all maxConcurrent slots taken). > 0 for any extended
	// period = the server is under-provisioned on HLS concurrency.
	TranscodeQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_transcode_queue_depth",
		Help: "Number of HLS session requests waiting for a concurrency slot.",
	})

	// TranscodeDuration tracks how long a transcode job ran before its
	// terminal transition (completed, failed, or cancelled). Default prom
	// buckets top out at 10s which is useless for transcodes that routinely
	// take minutes to hours. Custom buckets span seconds to two hours.
	TranscodeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "evey_transcode_duration_seconds",
		Help:    "Transcode job duration in seconds, by terminal status.",
		Buckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1200, 1800, 3600, 7200},
	}, []string{"status"})
)

func init() {
	prometheus.MustRegister(
		HLSSessionsGauge, TranscodesActive,
		HTTPRequestsTotal, HTTPRequestDuration,
		DBConnectionsOpen, DBConnectionsIdle, DBConnectionsInUse,
		MediaItemsTotal,
		AuthLatency, ScanQueueDepth, TranscodeQueueDepth,
		TranscodeDuration,
	)
}

// Handler returns the /metrics HTTP handler, restricted to loopback callers.
func Handler() http.Handler {
	ph := promhttp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ph.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// SanitizePath replaces high-cardinality path segments (IDs, UUIDs, tokens, numerics)
// with stable placeholders so Prometheus labels stay bounded.
//
// Rules per segment:
//   - purely numeric                       -> {id}
//   - typed prefix `xxx-...` (e.g. usr-ab) -> {id}
//   - hex-only of length >= 16             -> {id}
//   - UUID canonical form                  -> {id}
//   - length > 30 (likely token)           -> {token}
func SanitizePath(p string) string {
	if p == "" || p == "/" {
		return p
	}
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if seg == "" {
			continue
		}
		parts[i] = sanitizeSegment(seg)
	}
	return strings.Join(parts, "/")
}

func sanitizeSegment(seg string) string {
	if isAllDigits(seg) {
		return "{id}"
	}
	if isUUID(seg) {
		return "{id}"
	}
	if isTypedID(seg) {
		return "{id}"
	}
	if len(seg) > 30 {
		return "{token}"
	}
	if len(seg) >= 16 && isAllHex(seg) {
		return "{id}"
	}
	return seg
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isAllHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// isTypedID matches `xxx-...` where xxx is 2-5 lowercase letters and the suffix
// is 3+ alphanumeric chars. Examples: usr-abc123, med-xyz789.
func isTypedID(s string) bool {
	dash := strings.IndexByte(s, '-')
	if dash < 2 || dash > 5 {
		return false
	}
	prefix := s[:dash]
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		if c < 'a' || c > 'z' {
			return false
		}
	}
	suffix := s[dash+1:]
	if len(suffix) < 3 {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		c := suffix[i]
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// isUUID checks the canonical 8-4-4-4-12 hex form.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

// InstrumentHandler wraps h to record request count and duration.
func InstrumentHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		h.ServeHTTP(rec, r)
		elapsed := time.Since(start).Seconds()
		path := SanitizePath(r.URL.Path)
		HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rec.status)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(elapsed)
	})
}
