// Package cmcd parses Common Media Client Data per CTA-5004 (rev. 2020-09)
// from incoming segment requests. hls.js, Shaka, ExoPlayer and Safari (17+)
// all emit this payload natively when they fetch a segment - we just have to
// read it.
//
// Payload format (CTA-5004 §6.1):
//
//	GET /hls/<id>/stream_0_042.m4s?CMCD=bl%3D12000%2Cbr%3D2400%2Cmtp%3D8000%2Cot%3Dv%2Csid%3D%22...%22
//
// The query value is comma-separated key=value pairs (URL-encoded once). The
// same payload can also arrive as one of four headers (CMCD-Request,
// CMCD-Object, CMCD-Session, CMCD-Status) split by category - the merged shape
// is identical so we parse them uniformly.
//
// We deliberately keep the parser permissive: an unrecognized key is ignored
// (CTA-5004 allows custom keys), a malformed value never causes a 5xx - at
// worst the field stays at its zero value and the rest of the payload is
// still recorded. The QoE pipeline must never break playback.
package cmcd

import (
	"net/http"
	"strconv"
	"strings"
)

// Payload is the parsed view of a single segment request's CMCD data. Field
// names mirror CTA-5004 §5 keys. Pointers (or sentinel zero values) signal
// "absent" - players omit any key they don't have data for.
//
// We don't store every CTA-5004 key; only the ones with operational value
// for a self-hosted media server. The rest (nor, nrr, pr, ...) round-trip
// to Raw for future use.
type Payload struct {
	// Request scope - refreshes on every segment.
	BufferLengthMs int  // bl: client-side buffer length at request time
	DeadlineMs     int  // dl: deadline in ms from request to playback need
	MeasuredKbps   int  // mtp: throughput the client measured on the prev fetch
	BufferStarved  bool // bs: "buffer was starved since the last request"
	Startup        bool // su: this is the startup phase (first segment of session)

	// Object scope - about the segment being fetched.
	EncodedKbps    int    // br: bitrate of the rendition being downloaded
	ObjectType     string // ot: a|v|av|i|m|t|tt (audio, video, muxed, init, manifest, timed-text, trickplay)
	ObjectDuration int    // d: segment duration in ms
	TopBitrateKbps int    // tb: top rendition bitrate available to the client

	// Session scope - same across the whole playback session.
	SessionID       string // sid: UUID-ish, set by the player at session start
	ContentID       string // cid: opaque per-content key, set by the player
	StreamingFormat string // sf: "d" DASH | "h" HLS | "s" Smooth
	StreamType      string // st: "v" VOD | "l" live
	Version         int    // v: CMCD version

	// Status scope - operational signals.
	RequestedMaxKbps int     // rtp: cap the client has requested
	PlaybackRate     float64 // pr: 1.0 normal, 2.0 double speed, 0.5 half

	// Raw is the full key→value map exactly as parsed. Useful for future keys
	// we haven't promoted to typed fields, and for the SQL JSON column so we
	// don't drop data we'll want later.
	Raw map[string]string
}

// FromRequest extracts CMCD from query (?CMCD=...) first, then falls back to
// the four CMCD-* headers. Returns (zero, false) when no CMCD is present -
// callers must not treat that as an error (most legacy clients send nothing).
func FromRequest(r *http.Request) (Payload, bool) {
	if q := r.URL.Query().Get("CMCD"); q != "" {
		return Parse(q), true
	}
	parts := make([]string, 0, 4)
	for _, name := range []string{"CMCD-Request", "CMCD-Object", "CMCD-Session", "CMCD-Status"} {
		if v := r.Header.Get(name); v != "" {
			parts = append(parts, v)
		}
	}
	if len(parts) == 0 {
		return Payload{}, false
	}
	return Parse(strings.Join(parts, ",")), true
}

// parse decodes a comma-separated CMCD payload. Values can be:
//   - bare token (key only)           → boolean true
//   - integer                         → integer
//   - float                           → float
//   - quoted string ("..." with \")   → string
//   - other token                     → string (unquoted)
//
// Whitespace around keys/values is trimmed. Unknown keys land in Raw only.
// maxRawKeys bounds the number of distinct CTA-5004 keys we keep per
// payload. The spec is open - vendors are allowed custom keys - but
// a buggy / malicious player blasting thousands of synthetic keys
// would inflate the JSON we persist in playback_qoe_events.raw with
// no analytical value. 50 covers the standard keys (~12) plus a
// healthy margin for vendor extensions; beyond that, drop.
const maxRawKeys = 50

// maxValueLen bounds a single CTA-5004 value. maxRawKeys caps how many keys we
// persist; without this, one key could still carry a payload as large as the
// request allows, on every segment request of a long session. Every standard
// key is far shorter (a session id is a UUID, ~36 bytes), so 256 truncates only
// what was never meaningful.
const maxValueLen = 256

// Parse decodes one comma-separated CMCD payload. Exported for the native
// apps' QoE beacon: ExoPlayer and AVPlayer never attach CMCD to their own
// segment fetches, so the app posts the same key=value string on the
// player-event route instead, and the handler parses it like a header.
func Parse(s string) Payload {
	p := Payload{Raw: make(map[string]string, 12)}

	// Split honouring quoted strings: CTA-5004 allows commas inside quotes
	// (e.g. sid="abc,def" - rare but legal).
	for _, kv := range splitTopLevelCommas(s) {
		k, v := splitKV(strings.TrimSpace(kv))
		if k == "" {
			continue
		}
		// Standard keys always go through `assign` even past the cap
		// (the strongly-typed Payload fields drive the QoE dashboard).
		// Only the unstructured Raw map respects the cap so JSONB
		// storage stays bounded.
		if len(v) > maxValueLen {
			v = v[:maxValueLen]
		}
		if len(p.Raw) < maxRawKeys {
			p.Raw[k] = v
		}
		assign(&p, k, v)
	}
	return p
}

func splitTopLevelCommas(s string) []string {
	// Bound the slice a forged comma-heavy CMCD (up to the HTTP header/query
	// limit) can materialize on the segment hot path. A real payload has well
	// under ~30 keys; past maxParts the rest is junk we'd drop at the maxRawKeys
	// cap anyway, so stop building the slice instead of allocating all of it.
	const maxParts = 128
	var out []string
	var cur strings.Builder
	inQuotes := false
	for i := 0; i < len(s) && len(out) < maxParts; i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuotes = !inQuotes
			cur.WriteByte(c)
		case c == ',' && !inQuotes:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 && len(out) < maxParts {
		out = append(out, cur.String())
	}
	return out
}

func splitKV(s string) (key, val string) {
	before, after, ok := strings.Cut(s, "=")
	if !ok {
		// Boolean-style key (e.g. "bs", "su"): the bare presence means true.
		return s, ""
	}
	key = strings.TrimSpace(before)
	val = strings.TrimSpace(after)
	// Unquote if the value is a CMCD quoted string.
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = val[1 : len(val)-1]
		// CMCD permits \" inside quotes; unescape minimally.
		val = strings.ReplaceAll(val, `\"`, `"`)
	}
	return key, val
}

func assign(p *Payload, k, v string) {
	switch k {
	case "bl":
		p.BufferLengthMs = atoi(v)
	case "dl":
		p.DeadlineMs = atoi(v)
	case "mtp":
		p.MeasuredKbps = atoi(v)
	case "bs":
		p.BufferStarved = true
	case "su":
		p.Startup = true
	case "br":
		p.EncodedKbps = atoi(v)
	case "ot":
		p.ObjectType = v
	case "d":
		p.ObjectDuration = atoi(v)
	case "tb":
		p.TopBitrateKbps = atoi(v)
	case "sid":
		p.SessionID = v
	case "cid":
		p.ContentID = v
	case "sf":
		p.StreamingFormat = v
	case "st":
		p.StreamType = v
	case "v":
		p.Version = atoi(v)
	case "rtp":
		p.RequestedMaxKbps = atoi(v)
	case "pr":
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			p.PlaybackRate = f
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
