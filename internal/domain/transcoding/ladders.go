package transcoding

import "strings"

// ContentClass tags each media item with a per-title encoding hint computed
// at scan time from cheap source-side metrics (signalstats YDEV + scenecut
// rate + source bitrate + resolution). The hint maps to one of four
// pre-tuned ABR ladders chosen so the bandwidth/quality tradeoff matches
// the actual content — anime gets a 2-rung ladder peaking at 1.2 Mbps,
// action 4K HDR gets up to 15 Mbps. Mux's "Instant Per-Title Encoding"
// pattern, the simplified version of Netflix per-title that avoids the
// VMAF pre-encode pass.
//
// The class is purely advisory: profilesForDecision falls back to
// LadderNormal whenever the column is null, so unclassified items behave
// exactly like before the feature.
type ContentClass string

const (
	ClassAnimation     ContentClass = "animation"
	ClassFilmLowMotion ContentClass = "film_low_motion"
	ClassFilmNormal    ContentClass = "film_normal"
	ClassFilmHighMotion ContentClass = "film_high_motion"
	ClassSport         ContentClass = "sport"
)

// IsValid reports whether s is a known class. Unknown / empty values fall
// back to LadderNormal.
func (c ContentClass) IsValid() bool {
	switch c {
	case ClassAnimation, ClassFilmLowMotion, ClassFilmNormal, ClassFilmHighMotion, ClassSport:
		return true
	}
	return false
}

// ParseContentClass normalises an admin-typed value (case-insensitive, spaces
// stripped). Returns empty + ok=false on unknown.
func ParseContentClass(s string) (ContentClass, bool) {
	c := ContentClass(strings.ToLower(strings.TrimSpace(s)))
	if c.IsValid() {
		return c, true
	}
	return "", false
}

// LadderAnimation — light ladder for high-compressibility content (cell-shaded
// or flat colour anime, classic 2D). YDEV stays low, scenecuts rare, source
// bitrate typically <3 Mbps for a 1080p anime. Two rungs are enough because
// the per-frame bandwidth is so flat that ABR has little to switch between.
var LadderAnimation = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 400, AudioBitrate: 96},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 1200, AudioBitrate: 128},
}

// LadderFilmLowMotion — drama, romance, dialogue-driven, fixed cameras.
// YDEV moderate, scenecuts sparse. Bandwidth comfortable below the project
// default; ladder keeps the 1080p rung but cuts both lower rungs and the
// per-rung bitrate.
var LadderFilmLowMotion = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 600, AudioBitrate: 96},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1000, AudioBitrate: 128},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 2500, AudioBitrate: 192},
}

// LadderFilmNormal — pre-refactor default. Used as fallback when the
// classifier is undecided, when complexity_class is null (unclassified
// rows), and when an admin override forces it.
var LadderFilmNormal = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 128},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
}

// LadderFilmHighMotion — sustained action, blockbuster, anything that
// flirts with the 4K HDR territory. The per-rung bitrates climb so the
// hard cases (sustained explosions, fast pans, grain) don't crater
// perceptual quality.
var LadderFilmHighMotion = []HLSProfile{
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1500, AudioBitrate: 128},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3500, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 7000, AudioBitrate: 192},
	{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 15000, AudioBitrate: 256},
}

// LadderSport — broadcast-style continuous high motion (sports, esports,
// live events). Same shape as high-motion but with a slightly heavier
// 720p rung because sports viewers watching on mobile still want sharp
// motion. Rare in a self-hosted film/TV library — kept for completeness.
var LadderSport = []HLSProfile{
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1800, AudioBitrate: 128},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 4500, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192},
}

// LadderFor returns the ABR ladder pre-tuned for the given class. Empty or
// unknown ⇒ LadderFilmNormal (safe fallback that matches pre-refactor
// behaviour, so a non-classified library still serves the same playlist
// it did yesterday).
func LadderFor(c ContentClass) []HLSProfile {
	switch c {
	case ClassAnimation:
		return LadderAnimation
	case ClassFilmLowMotion:
		return LadderFilmLowMotion
	case ClassFilmHighMotion:
		return LadderFilmHighMotion
	case ClassSport:
		return LadderSport
	default:
		return LadderFilmNormal
	}
}

// CapLadderToHeight trims the ladder so no rung exceeds maxHeight pixels.
// Designed for the software-only encode path: ffmpeg software encoders
// can't keep up with a 2160p H.264 rung on a 4K HEVC source — the whole
// pipeline (which produces every variant in a single ffmpeg via
// var_stream_map) gets bottlenecked by the heaviest rung. Capping to
// 1080p (maxHeight=1080) is what every other OSS player (Jellyfin,
// Stash, Navidrome) does when no HW accelerator is available.
//
// maxHeight=0 ⇒ no cap.
// Always returns at least one rung (the lowest).
func CapLadderToHeight(ladder []HLSProfile, maxHeight int) []HLSProfile {
	if maxHeight <= 0 {
		return ladder
	}
	out := make([]HLSProfile, 0, len(ladder))
	for _, p := range ladder {
		if p.Height <= maxHeight {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return ladder[:1]
	}
	return out
}

// CapLadderToSource trims the returned ladder so no rung tries to upscale
// beyond the source resolution — pointless bandwidth and risks visible
// blurring. e.g. a 720p source with LadderFilmHighMotion (which has a
// 2160p rung) gets clipped to 480p/720p only.
//
// sourceWidth/Height come from the source media stream metadata. Zero or
// negative ⇒ no clipping (treat as unknown).
func CapLadderToSource(ladder []HLSProfile, sourceWidth, sourceHeight int) []HLSProfile {
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return ladder
	}
	out := make([]HLSProfile, 0, len(ladder))
	for _, p := range ladder {
		// Allow the rung when it's strictly smaller OR equal in the tallest
		// dimension. Some sources are letterboxed (e.g. 1920×800) — cap
		// against the larger of width/height vs the rung's larger side.
		srcMax := sourceWidth
		if sourceHeight > srcMax {
			srcMax = sourceHeight
		}
		rungMax := p.Width
		if p.Height > rungMax {
			rungMax = p.Height
		}
		if rungMax <= srcMax {
			out = append(out, p)
		}
	}
	// Always keep at least one rung — if the source is tiny (<360p), give
	// the lowest pre-defined rung anyway so the player has something.
	if len(out) == 0 {
		return ladder[:1]
	}
	return out
}
