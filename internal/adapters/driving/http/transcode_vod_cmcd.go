package httpapi

// transcode_vod_cmcd.go ingests CMCD query parameters from segment
// requests and forwards playback hints to the stream producer.

import (
	"log/slog"
	"net/http"

	appqoe "github.com/evey-project/evey/internal/app/qoe"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/pkg/cmcd"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
)

// ingestCMCD parses CMCD (CTA-5004) from the request and forwards the event
// to the QoE sink. No-op when CMCD is absent or the sink isn't wired (dev /
// tests). Resolves user_id from the request context - anonymous plays
// (shared link, no token) end up with an empty user_id, which is fine.
func (h *hlsVODServe) ingestCMCD(r *http.Request, mediaID string, variantIdx, segmentIdx int, isInit, isManifest, isVariantPL bool) {
	if h.qoeSink == nil {
		return
	}
	payload, ok := cmcd.FromRequest(r)
	if !ok {
		return
	}
	eveymetrics.QoEIngestEvents.Inc()

	var userID string
	if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
		userID = claims.UserID
	}

	// Debug-level: CMCD ingest fires once per beacon (per segment, per stream),
	// so Info would flood prod logs. The Phase 1 rollout is over - the QoE SQL
	// table and the /admin/qoe dashboard are confirmed working - so this drops
	// from Info to Debug (kept for on-demand troubleshooting, not steady-state).
	slog.Debug("hls-vod: CMCD ingest",
		slog.String("media_id", mediaID),
		slog.String("user_id", userID),
		slog.Int("variant", variantIdx),
		slog.Int("seg", segmentIdx),
		slog.Bool("init", isInit),
		slog.Bool("manifest", isManifest),
		slog.Bool("variant_pl", isVariantPL),
		slog.String("sid", payload.SessionID),
		slog.Int("buffer_ms", payload.BufferLengthMs),
		slog.Int("encoded_kbps", payload.EncodedKbps),
		slog.Int("measured_kbps", payload.MeasuredKbps),
		slog.Bool("starved", payload.BufferStarved),
		slog.Bool("startup", payload.Startup))

	h.qoeSink.Record(appqoe.Event{
		UserID:      userID,
		MediaItemID: mediaID,
		VariantIdx:  variantIdx,
		SegmentIdx:  segmentIdx,
		IsInit:      isInit,
		IsManifest:  isManifest,
		IsVariantPL: isVariantPL,
		DeviceClass: appqoe.DeviceClass(r.UserAgent()),
		RemoteIP:    appqoe.RemoteIP(r.RemoteAddr),
		Payload:     payload,
	})
}

// notifyProducerHint forwards CMCD telemetry (buffer length, starvation flag,
// current segment cursor) to the resolved producer's NotifyClientHint so the
// throttle / prefetch / pacing policy can react in real time. Read-only on the
// HTTP path - never blocks, never errors. No-op when CMCD is absent or no
// producer resolved.
//
// NEVER touches which variant is served - the player owns rendition
// selection. The feedback loop only adjusts encoder pacing.
func (h *hlsVODServe) notifyProducerHint(producer apptranscoding.StreamProducer, r *http.Request, mediaID string, variantIdx, segmentIdx int) {
	if producer == nil {
		return
	}
	payload, ok := cmcd.FromRequest(r)
	if !ok {
		return
	}
	// Bare-minimum signals: buffer length (ms) and starvation flag drive
	// the policy. Object type "v"/"av" matters most (video pipeline),
	// audio-only fetches can still bump clientHighWater forward harmlessly.
	// variantIdx scopes the hint to the rung being fetched: without it a hint
	// could resume or un-throttle the rung the player just left.
	producer.NotifyClientHint(mediaID, variantIdx, payload.BufferLengthMs, payload.BufferStarved, segmentIdx)
}
