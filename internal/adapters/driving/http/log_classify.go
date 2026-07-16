package httpapi

import "strings"

// Log "where" (subsystem) categories surfaced as admin filter options. Kept in
// sync with the frontend option list (admin.logs.where.* i18n keys).
const (
	wherePlayer     = "player"
	whereSeekIndex  = "seek-index"
	whereSubtitle   = "subtitle"
	whereRemux      = "remux"
	whereHWDetect   = "hw-detect"
	whereIntro      = "intro"
	whereTrickplay  = "trickplay"
	whereQoE        = "qoe"
	whereChromecast = "chromecast"
	whereAuth       = "auth"
	whereInfra      = "infra"
	whereOther      = "other"
)

// Log "source" categories: what produced the line. Within where=player the
// operator uses this to isolate ffmpeg vs mkvgo (or leave blank for both).
const (
	sourceFFmpeg = "ffmpeg"
	sourceMkvgo  = "mkvgo"
	sourceQoE    = "qoe"
	sourceGRPC   = "grpc"
	sourceOther  = "other"
)

// ffmpegComponents / mkvgoComponents drive the ffmpeg vs mkvgo split. The big
// ffmpeg adapter sets no component and is matched by msg prefix instead.
var ffmpegComponents = map[string]bool{
	"software_producer":          true,
	"segment_orchestrator":       true,
	"audio_segment_orchestrator": true,
	"keyframe_backfill_worker":   true,
	"level_backfill_worker":      true,
	"complexity_worker":          true,
}

var mkvgoComponents = map[string]bool{
	"seek_index": true,
	"seek-index": true, // gRPC service spells it kebab-case; worker uses snake.
}

// classifyLog derives (where, source) from a record's component + msg. It is
// deterministic and order-sensitive: component rules win over msg-prefix rules.
func classifyLog(component, msg string) (where, source string) {
	return classifyWhere(component, msg), classifySource(component, msg)
}

func classifySource(component, msg string) string {
	if ffmpegComponents[component] {
		return sourceFFmpeg
	}
	if mkvgoComponents[component] {
		return sourceMkvgo
	}
	switch component {
	case "qoe_sink":
		return sourceQoE
	case "grpc":
		return sourceGRPC
	}
	m := strings.ToLower(msg)
	switch {
	case hasAnyPrefix(m, "hls:", "software:", "transcode:", "transcoder", "ffmpeg", "hwdetect:", "benchmark:", "av1 decoder probe", "no av1 decoder"):
		return sourceFFmpeg
	case strings.Contains(m, "mkvgo") || strings.HasPrefix(m, "seek-index"):
		return sourceMkvgo
	}
	return sourceOther
}

func classifyWhere(component, msg string) string {
	// Component-based rules first (most reliable).
	switch component {
	case "software_producer", "segment_orchestrator", "audio_segment_orchestrator":
		return wherePlayer
	case "keyframe_backfill_worker", "level_backfill_worker", "complexity_worker", "remux_cache_evict", "segment_cache_evict":
		return whereInfra
	case "seek_index", "seek-index":
		return whereSeekIndex
	case "player_client":
		// The playback-error beacon: what the PLAYER saw. It belongs with the
		// server's own player lines - an operator investigating an incident
		// must find both under one filter.
		return wherePlayer
	case "qoe_sink":
		return whereQoE
	case "scheduler", "webhook", "backup-scheduler", "grpc", "watcher", "metrics_sampler":
		return whereInfra
	}

	m := strings.ToLower(msg)
	switch {
	case hasAnyPrefix(m, "hls:", "software:", "transcode:"):
		return wherePlayer
	case strings.HasPrefix(m, "subtitle"):
		// Checked before the broad mkvgo match: subtitle extraction lines
		// mention mkvgo but belong to the subtitle subsystem.
		return whereSubtitle
	case strings.HasPrefix(m, "remux"):
		return whereRemux
	case strings.HasPrefix(m, "seek-index") || strings.Contains(m, "mkvgo"):
		return whereSeekIndex
	case hasAnyPrefix(m, "hwdetect:", "benchmark:", "av1 decoder probe", "no av1 decoder"):
		return whereHWDetect
	case strings.Contains(m, "intro_detection") || strings.HasPrefix(m, "intro"):
		return whereIntro
	case strings.Contains(m, "trickplay"):
		return whereTrickplay
	case hasAnyPrefix(m, "chromecast", "devices command"):
		return whereChromecast
	case hasAnyPrefix(m, "oidc:", "csrf") || strings.Contains(m, "token reuse") || strings.HasPrefix(m, "stream token"):
		return whereAuth
	}
	return whereOther
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
