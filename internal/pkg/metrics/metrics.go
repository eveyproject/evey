package metrics

import (
	"bufio"
	"fmt"
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

	// AuthFailuresTotal counts rejected authentications on the generic API
	// routes. The login form logs per attempt (user/command_service.go), but a
	// credential-stuffing run replaying stolen bearers against the API was
	// invisible: 401 with no trace. reason: "missing_bearer" (probing without
	// any credential) vs "invalid_token" (a credential was presented and
	// rejected - the security-relevant bucket to alert on).
	AuthFailuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_auth_failures_total",
		Help: "Rejected authentications on generic API routes.",
	}, []string{"reason"})

	// AuthzDeniedTotal counts callers who authenticated fine but were refused
	// by an authorization rule. Distinct from AuthFailuresTotal above, which is
	// about credentials: this one is "who you are is not enough for this".
	//
	// It exists because both admin-RBAC enforcement points (gRPC interceptor and
	// the HTTP admin middleware) refused silently - no log, no metric - while
	// every neighbouring path logged. That is the same shape as the 2026-07-29
	// remote-access lockout: a refused client and zero server-side signal, which
	// is what made it take two days to find. surface: "grpc" | "http".
	AuthzDeniedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_authz_denied_total",
		Help: "Authenticated callers refused by an authorization rule.",
	}, []string{"surface", "reason"})

	// RemoteAccessRefusedTotal counts clients turned away by the remote-access
	// network gate. The refusal is logged (rate limited per address), but volume
	// over time is what tells an operator "my whole household is locked out"
	// rather than "one client had a bad day", and grep does not alert.
	RemoteAccessRefusedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_remote_access_refused_total",
		Help: "Requests refused because remote access is disabled.",
	})

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

	// WatcherProbeTotal counts inotify liveness probe outcomes per library.
	// The probe writes a file into a watched directory and waits for its own
	// event; result is "alive", "dead" or "inconclusive" (could not write - a
	// read-only root, which is a legitimate setup, not a broken watch).
	//
	// This is the metric to look at first when someone reports "new media does
	// not appear": a rising "dead" count means real-time monitoring is down and
	// the server is relying on the periodic walk alone.
	WatcherProbeTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_watcher_probe_total",
		Help: "inotify liveness probe outcomes (alive|dead|inconclusive) per library.",
	}, []string{"library_id", "result"})

	// WatcherAlive is 1 when a library's watch answered its last probe, 0 when
	// it did not. Unlike the counter above this survives as a current-state
	// signal, so a dashboard can show which libraries are being watched right
	// now versus which are falling back to periodic scanning.
	WatcherAlive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "evey_watcher_alive",
		Help: "1 if the library's inotify watch answered its last liveness probe, 0 otherwise.",
	}, []string{"library_id"})

	// LibraryScanTotal counts full library walks by why they ran: "startup",
	// "periodic" (the watch could not be vouched for), "watched_backstop" (the
	// watch is alive, this is the spaced-out safety net) or "probe_recovery"
	// (a probe just failed, catching up on what the dead watch missed).
	//
	// A library whose scans are all "periodic" is one whose watch never works -
	// exactly the diagnosis that used to require guessing.
	LibraryScanTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_library_scan_total",
		Help: "Full library scans by trigger reason.",
	}, []string{"library_id", "reason"})

	// TranscodeQueueDepth counts transcode jobs persisted as Queued because
	// every concurrency slot was taken (running >= maxConcurrent) when they were
	// requested. Fed by Service.refreshActiveGauge. > 0 for any extended period
	// = the server is under-provisioned for its transcode-job load.
	//
	// Note: this covers the explicit transcode-job path, not on-demand HLS,
	// which is admission-shaped (it sheds/spills rather than queueing) and does
	// not write these rows - see docs/transcode-saga-plan.md.
	TranscodeQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_transcode_queue_depth",
		Help: "Number of transcode jobs waiting for a concurrency slot (queued).",
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

	// ---- HLS VOD pipeline (Phase 1+ refacto) ----
	// These metrics live next to the legacy HLSSessionsGauge so a dashboard
	// can chart "old vs new" during the transition. After the legacy path is
	// retired (Phase 4) the legacy gauges go away.

	// HLSVODSessions counts ffmpeg rafales currently owned by the
	// SegmentOrchestrator. Distinct from HLSSessionsGauge which tracks the
	// legacy HLSManager - both can be >0 simultaneously during the cutover.
	HLSVODSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_hlsvod_sessions_active",
		Help: "Current number of active VOD-pipeline ffmpeg rafale sessions.",
	})

	// HLSVODRafaleSpawned counts every fresh ffmpeg rafale start. Tagged by
	// reason - "miss" (no covering session) or "seek_out_of_window" (the
	// previous rafale was killed by cancel-on-seek before this spawn).
	HLSVODRafaleSpawned = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_hlsvod_rafale_spawned_total",
		Help: "Total VOD-pipeline ffmpeg rafale spawns, by reason.",
	}, []string{"reason"})

	// HLSVODTranscodeSessions counts masters served by the ffmpeg TRANSCODE
	// ladder, by cause (TranscodeCause): what keeps a title off the free copy path.
	HLSVODTranscodeSessions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_hlsvod_transcode_sessions_total",
		Help: "Masters served by the transcode ladder, by the cause that ruled out a copy.",
	}, []string{"cause"})

	// HLSVODCopyTimelineDrift counts copy-rung segments whose rebased fMP4
	// timeline does NOT match the keyframe plan's position for that segment
	// index (beyond tolerance). This is the runtime tripwire for the bug class
	// that produced the 2026-06-10 A/V desyncs (session-relative muxer cuts,
	// seek landings off the requested keyframe, trimmed-but-mislabeled audio):
	// any future regression of the plan↔ffmpeg contract trips this counter and
	// an ERROR log instead of surfacing as audible desync. Must stay at zero.
	HLSVODCopyTimelineDrift = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_copy_timeline_drift_total",
		Help: "Copy-rung segments whose produced timeline diverged from the keyframe plan (must be 0).",
	})

	// HLSVODTranscodeSeekTimelineDrift is the transcode-seek sibling of
	// HLSVODCopyTimelineDrift: a mid-stream transcode session emits fixed-length
	// segments, so the same fMP4 rebase pump runs but was previously unverified.
	// A non-zero value means a rebased transcode-seek segment landed off its
	// linear expected position (idx × segment length) beyond tolerance. Must
	// stay at zero; kept separate from the copy counter so an alert points at
	// the right pipeline.
	HLSVODTranscodeSeekTimelineDrift = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_transcode_seek_timeline_drift_total",
		Help: "Transcode-seek segments whose rebased timeline diverged from the expected linear position (must be 0).",
	})

	// HLSVODSessionsKilled counts session terminations, tagged by reason
	// (seek_out_of_window, inactivity, close, error). Useful to spot
	// pathologies - e.g. inactivity kills > 0 means the inactivity timeout
	// is shorter than the player's natural pause window.
	HLSVODSessionsKilled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_hlsvod_sessions_killed_total",
		Help: "Total VOD-pipeline ffmpeg session kills, by reason.",
	}, []string{"reason"})

	// HLSVODSessionsCompleted counts sessions that ended on their OWN ffmpeg EOF
	// (the dominant ~95% path) rather than a forced kill (HLSVODSessionsKilled).
	// The two are mutually exclusive, so spawns − (completed + killed) should
	// equal the live HLSVODSessions gauge; a persistent divergence flags a
	// teardown/gauge leak (the failure mode the per-session sync.Once was added
	// to fix - previously natural exits were unmetered and invisible).
	HLSVODSessionsCompleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_sessions_completed_total",
		Help: "Total VOD-pipeline ffmpeg sessions that ended on natural EOF (not a forced kill).",
	})

	// HLSVODSessionsRejected counts spawn refusals when the orchestrator is at
	// its MaxActiveSessions global cap. A sustained nonzero rate means the host
	// is shedding transcode load (503 + Retry-After to the player) - either
	// genuine overload on a weak host or an abusive hash-fanout client.
	HLSVODSessionsRejected = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_sessions_rejected_total",
		Help: "Total VOD-pipeline rafale spawns refused due to the global session cap.",
	})

	// HLSVODRouteNotExpressible counts the times a resolved GPU route reached
	// the ffmpeg builder and the builder had no pipeline for it (it then fell
	// back to a software encode under a GPU-billed session). Every hit is a
	// planner/builder disagreement - an ADR-0030 invariant violation - and the
	// counter must read zero before the builder is switched from fallback to
	// refusal.
	HLSVODRouteNotExpressible = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_route_not_expressible_total",
		Help: "Total resolved GPU routes the ffmpeg builder could not express (planner/builder disagreement).",
	})

	// HLSVODRouteDrift counts the times a live stream's route (codec, backend,
	// topology) re-resolved to something else while its admission pin was
	// still authoritative: the capability, a demotion or a policy toggle moved
	// under a playing session. The player is served a 410 on its sealed URLs
	// and refetches the master; this counter is the direct detector of how
	// often that happens (ADR-0030: the route is not frozen on the pin, by
	// choice - a demoted backend must not be kept alive by it).
	HLSVODRouteDrift = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_route_drift_total",
		Help: "Total live streams whose route re-resolved differently while their admission pin was still authoritative.",
	})

	// HLSVODDeliveryDisagreements counts the masters the HTTP adapter served
	// on a delivery path (which engine carried the video, the audio) other
	// than the one the plan DECIDED (PlaybackPlan.Delivery). Shadow of the
	// hand-over of that choice to the plan: it must read zero before the
	// adapter stops choosing engines itself; afterwards every hit is an
	// adapter that re-decided, a bug.
	HLSVODDeliveryDisagreements = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_hlsvod_delivery_disagreements_total",
		Help: "Masters served on a delivery path other than the plan's (planned vs served engines).",
	}, []string{"planned", "served"})

	// HLSVODCacheHits / HLSVODCacheMisses split segment requests by whether
	// they hit the disk cache before waiting for ffmpeg. Hit-rate >> 0 is
	// the primary KPI for the multi-session cache: low hit-rate means users
	// rarely share segments.
	HLSVODCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_cache_hits_total",
		Help: "Total segment requests served straight from the disk cache.",
	})
	HLSVODCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_cache_misses_total",
		Help: "Total segment requests that had to wait for ffmpeg to land the segment.",
	})

	// HLSVODCacheBytes tracks the total on-disk segment cache size.
	// Updated by the eviction sweeper on each pass.
	HLSVODCacheBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "evey_hlsvod_cache_size_bytes",
		Help: "Current total VOD segment cache size on disk.",
	})

	// HLSVODCacheEvictedBytes counts bytes freed by the LRU sweeper.
	// Sustained > 0 means the cache is undersized for the workload.
	HLSVODCacheEvictedBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_cache_evicted_bytes_total",
		Help: "Total bytes freed from the VOD segment cache by the LRU sweeper.",
	})

	// HLSVODThrottlePause / HLSVODThrottleResume count adaptive throttle
	// state transitions. Sustained Pause >> Resume means clients are mostly
	// idle on linear playback (good - CPU saved). The ratio also indicates
	// whether the PauseAhead / ResumeBelow thresholds are well-tuned: lots
	// of pair churn on a single session = flap, widen the hysteresis.
	HLSVODThrottlePause = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_throttle_pause_total",
		Help: "Total ffmpeg SIGSTOPs issued by the pause-when-ahead policy.",
	})
	HLSVODThrottleResume = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_throttle_resume_total",
		Help: "Total ffmpeg SIGCONTs issued by the pause-when-ahead policy.",
	})
	// HLSVODStarvationResumes counts only the resumes triggered by CMCD
	// bs=1 (vs the regular tick-based ones). High value vs total resume
	// means the throttle tick is too slow for the network.
	HLSVODStarvationResumes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_hlsvod_starvation_resumes_total",
		Help: "ffmpeg SIGCONTs triggered by CMCD bs=1 starvation signal.",
	})

	// QoEIngestEvents counts every CMCD event received. QoEIngestDrops counts
	// the ones the sink dropped because its buffer was full - sustained > 0
	// means we need a bigger buffer or a faster DB.
	QoEIngestEvents = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_qoe_ingest_events_total",
		Help: "Total CMCD (CTA-5004) events received from segment requests.",
	})
	QoEIngestDrops = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "evey_qoe_ingest_drops_total",
		Help: "CMCD events dropped due to sink buffer overflow.",
	})

	// RateLimitRejected counts requests refused by the per-method per-IP rate
	// limiter, labeled by gRPC method. It is the primary ops signal for a
	// brute-force / credential-stuffing attempt in progress: the limiter runs
	// BEFORE the logging interceptor, so without this counter (and the paired
	// slog.Warn) a throttled attack is invisible. Method cardinality is bounded by
	// the fixed DefaultPublicMethodLimiters set.
	RateLimitRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_ratelimit_rejected_total",
		Help: "Requests rejected by the per-method per-IP rate limiter, by method.",
	}, []string{"method"})

	// PluginUp is 1 while a plugin's last known status is Running, 0 while it
	// is Error or stopped. The plugin subsystem previously had no metric at
	// all: weather-v2 sat in Error for 3 days in prod with only a log line and
	// a DB column to notice it - nothing to alert on.
	PluginUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "evey_plugin_up",
		Help: "1 if the plugin's last known status is Running, 0 if Error or stopped.",
	}, []string{"plugin_id"})

	// PluginErrorsTotal counts every transition of a plugin into Error status
	// (spawn failure, IPC failure, auto-start retry failure). A rising count
	// with a flat PluginUp=0 for the same plugin_id is the retry loop that
	// used to be invisible.
	PluginErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "evey_plugin_errors_total",
		Help: "Total transitions of a plugin into Error status, by plugin_id.",
	}, []string{"plugin_id"})
)

// SetPluginUp records whether the given plugin is currently up (Running) or
// not (Error/stopped).
func SetPluginUp(id string, up bool) {
	v := 0.0
	if up {
		v = 1.0
	}
	PluginUp.WithLabelValues(id).Set(v)
}

// IncPluginError records a plugin transitioning into Error status.
func IncPluginError(id string) {
	PluginErrorsTotal.WithLabelValues(id).Inc()
}

func init() {
	prometheus.MustRegister(
		HLSVODTranscodeSessions,
		HLSSessionsGauge, TranscodesActive,
		HTTPRequestsTotal, HTTPRequestDuration, AuthFailuresTotal,
		AuthzDeniedTotal, RemoteAccessRefusedTotal,
		DBConnectionsOpen, DBConnectionsIdle, DBConnectionsInUse,
		MediaItemsTotal,
		AuthLatency, ScanQueueDepth, TranscodeQueueDepth,
		WatcherProbeTotal, WatcherAlive, LibraryScanTotal,
		TranscodeDuration,
		HLSVODSessions, HLSVODRafaleSpawned, HLSVODSessionsKilled, HLSVODSessionsCompleted, HLSVODSessionsRejected,
		HLSVODRouteNotExpressible, HLSVODRouteDrift, HLSVODDeliveryDisagreements,
		HLSVODCopyTimelineDrift, HLSVODTranscodeSeekTimelineDrift,
		HLSVODCacheHits, HLSVODCacheMisses, HLSVODCacheBytes, HLSVODCacheEvictedBytes,
		HLSVODThrottlePause, HLSVODThrottleResume, HLSVODStarvationResumes,
		QoEIngestEvents, QoEIngestDrops,
		RateLimitRejected,
		PluginUp, PluginErrorsTotal,
	)
}

// Handler returns the /metrics HTTP handler, restricted to loopback callers.
func Handler() http.Handler {
	ph := promhttp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if h, _, ok := strings.Cut(host, "%"); ok {
			host = h
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

// Hijack forwards to the underlying writer: the websocket upgrade
// (/api/v1/devices/ws) type-asserts http.Hijacker on THIS wrapper and used to
// panic otherwise, killing Play On device registration behind InstrumentHandler.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("metrics: underlying ResponseWriter (%T) does not support hijacking", r.ResponseWriter)
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
