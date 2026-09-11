package transcoding

import "fmt"

// ContentClass tags each media item with a per-title encoding hint computed
// at scan time from cheap source-side metrics (signalstats YDEV + scenecut
// rate + source bitrate + resolution). The hint maps to one of four
// pre-tuned ABR ladders chosen so the bandwidth/quality tradeoff matches
// the actual content - anime gets a 2-rung ladder peaking at 1.2 Mbps,
// action 4K HDR gets up to 15 Mbps. Mux's "Instant Per-Title Encoding"
// pattern, the simplified version of Netflix per-title that avoids the
// VMAF pre-encode pass.
//
// The class is purely advisory: profilesForDecision falls back to
// LadderNormal whenever the column is null, so unclassified items behave
// exactly like before the feature.
type ContentClass string

const (
	ClassAnimation      ContentClass = "animation"
	ClassFilmLowMotion  ContentClass = "film_low_motion"
	ClassFilmNormal     ContentClass = "film_normal"
	ClassFilmHighMotion ContentClass = "film_high_motion"
	ClassSport          ContentClass = "sport"
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

// LadderAnimation - light ladder for high-compressibility content (cell-shaded
// or flat colour anime, classic 2D). YDEV stays low, scenecuts rare, source
// bitrate typically <3 Mbps for a 1080p anime. Two rungs are enough because
// the per-frame bandwidth is so flat that ABR has little to switch between.
// Audio floors across every ladder below, revised 2026-09-04. A rung's audio
// figure is quoted for STEREO, and a stereo rung on a multichannel title is a
// FOLD: two channels carry dialogue, music, effects and both surrounds at once,
// which is the opposite of the per-channel arithmetic that sizes a 5.1 encode.
// Measured on an AC-3 5.1 film, coding noise under the signal on a quiet
// ambience passage (the first thing to suffer, -38 dBFS): 21.6 dB at 96k,
// 26.7 at 128k, 31.9 at 160k, 35.5 at 192k, 38.4 at 256k. The return per
// kilobit collapses past 192k, so that is the ceiling for anything but 2160p;
// 128k is the floor kept on the lowest rung, where the whole budget is 400-800
// kbps of video and 64 extra kbps would be felt.
var LadderAnimation = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 400, AudioBitrate: 128},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 1200, AudioBitrate: 192},
}

// LadderFilmLowMotion - drama, romance, dialogue-driven, fixed cameras.
// YDEV moderate, scenecuts sparse. Bandwidth comfortable below the project
// default; ladder keeps the 1080p rung but cuts both lower rungs and the
// per-rung bitrate.
var LadderFilmLowMotion = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 600, AudioBitrate: 128},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1000, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 2500, AudioBitrate: 192},
}

// LadderFilmNormal - pre-refactor default. Used as fallback when the
// classifier is undecided, when complexity_class is null (unclassified
// rows), and when an admin override forces it.
var LadderFilmNormal = []HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 128},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1400, AudioBitrate: 192},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
}

// LadderFilmHighMotion - sustained action, blockbuster, anything that
// flirts with the 4K HDR territory. The per-rung bitrates climb so the
// hard cases (sustained explosions, fast pans, grain) don't crater
// perceptual quality.
var LadderFilmHighMotion = []HLSProfile{
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1500, AudioBitrate: 192},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 3500, AudioBitrate: 192},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 7000, AudioBitrate: 192},
	{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 15000, AudioBitrate: 256},
}

// LadderSport - broadcast-style continuous high motion (sports, esports,
// live events). Same shape as high-motion but with a slightly heavier
// 720p rung because sports viewers watching on mobile still want sharp
// motion. Rare in a self-hosted film/TV library - kept for completeness.
var LadderSport = []HLSProfile{
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1800, AudioBitrate: 192},
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
// can't keep up with a 2160p H.264 rung on a 4K HEVC source - the whole
// pipeline (which produces every variant in a single ffmpeg via
// var_stream_map) gets bottlenecked by the heaviest rung. Capping to
// 1080p (maxHeight=1080) is what the OSS field converges on when no HW
// accelerator is available.
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

// BoostCappedTopRung spends the bitrate budget of a capability-dropped rung at
// the height the encoder CAN sustain. When the height ceiling (software /
// software-HDR sustainable cap) removed rungs the source could have fed, the
// client was ready to budget the dropped rung's bandwidth anyway - so the top
// surviving rung adopts the FIRST dropped rung's video bitrate (e.g. a 4K HDR
// source capped to 720p serves 720p at the 1080p rung's 5000k instead of
// 2800k: the grain survives the downscale). First dropped, not highest: the
// next rung up is the budget the viewer actually lost. Raising maxrate at a
// fixed capped-CRF costs the encoder almost nothing - the cap is a bit budget,
// not a work order - so this stays inside the calibrated margin.
//
// heightCapped must be the CapLadderToHeight result of sourceCapped (an
// ascending prefix, or the single-lowest-rung fallback). Source-resolution
// trimming must happen BEFORE (in sourceCapped): a rung dropped because the
// source is small is not a quality loss and earns no boost.
//
// Never mutates its inputs (heightCapped may alias a package-level ladder).
func BoostCappedTopRung(sourceCapped, heightCapped []HLSProfile) []HLSProfile {
	if len(heightCapped) == 0 || len(heightCapped) >= len(sourceCapped) {
		return heightCapped
	}
	dropped := sourceCapped[len(heightCapped)]
	top := heightCapped[len(heightCapped)-1]
	if dropped.VideoBitrate <= top.VideoBitrate {
		return heightCapped
	}
	out := make([]HLSProfile, len(heightCapped))
	copy(out, heightCapped)
	out[len(out)-1].VideoBitrate = dropped.VideoBitrate
	return out
}

// CapLadderToSource trims the returned ladder so no rung tries to upscale
// beyond the source resolution - pointless bandwidth and risks visible
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
		// dimension. Some sources are letterboxed (e.g. 1920×800) - cap
		// against the larger of width/height vs the rung's larger side.
		srcMax := max(sourceHeight, sourceWidth)
		rungMax := max(p.Height, p.Width)
		if rungMax <= srcMax {
			out = append(out, p)
		}
	}
	// Always keep at least one rung - if the source is tiny (<360p), give
	// the lowest pre-defined rung anyway so the player has something.
	if len(out) == 0 {
		return ladder[:1]
	}
	return out
}

// HEVC needs about two thirds of the H.264 bitrate at equal quality.
const (
	HEVCBitrateNum = 2
	HEVCBitrateDen = 3
)

// ScaleLadderVideoBitrates multiplies every rung's video bitrate by num/den.
// RaiseLadderToSource lifts each rung's video bitrate to the source's, scaled
// by pixel count, so a 70 Mb/s UHD remux is not squeezed into 15 Mb/s of H.264
// (a remux deserves its own bitrate). Never lowers a rung; ceilingKbps
// (0 = none) is the client/profile/server cap.
func RaiseLadderToSource(ladder []HLSProfile, sourceKbps int64, sourceWidth, sourceHeight int, ceilingKbps int64) []HLSProfile {
	srcPx := int64(sourceWidth) * int64(sourceHeight)
	if sourceKbps <= 0 || srcPx <= 0 {
		return ladder
	}
	out := make([]HLSProfile, len(ladder))
	for i, p := range ladder {
		want := sourceKbps * int64(p.Width) * int64(p.Height) / srcPx
		if ceilingKbps > 0 && want > ceilingKbps {
			want = ceilingKbps
		}
		if want > p.VideoBitrate {
			p.VideoBitrate = want
		}
		out[i] = p
	}
	return out
}

// CapLadderToScreen drops the rungs above the smallest one that still covers
// the viewer's screen: a 768 px laptop gets up to 1080p, never a 2160p it
// would only downscale. A screen taller than every rung changes nothing.
func CapLadderToScreen(ladder []HLSProfile, screenHeight int) []HLSProfile {
	if screenHeight <= 0 {
		return ladder
	}
	limit := 0
	for _, p := range ladder {
		if p.Height >= screenHeight && (limit == 0 || p.Height < limit) {
			limit = p.Height
		}
	}
	if limit == 0 {
		return ladder
	}
	return CapLadderToHeight(ladder, limit)
}

func ScaleLadderVideoBitrates(ladder []HLSProfile, num, den int64) []HLSProfile {
	if den <= 0 || num <= 0 || num == den {
		return ladder
	}
	out := make([]HLSProfile, len(ladder))
	for i, p := range ladder {
		p.VideoBitrate = p.VideoBitrate * num / den
		out[i] = p
	}
	return out
}

// H264LevelIdcFor is the level_idc a rung of this height is signalled at: 4.1
// up to 1080p, 5.1 above (4.1's macroblock budget stops at 1080p).
func H264LevelIdcFor(height int) int {
	if height > 1080 {
		return 51
	}
	return 41
}

// H264LevelString formats a level_idc for ffmpeg ("41" -> "4.1").
func H264LevelString(idc int) string {
	return fmt.Sprintf("%d.%d", idc/10, idc%10)
}

// H264LevelMaxHeight is the tallest rung a decoder capped at this level_idc
// decodes (frame-size limits of the H.264 level table). 0 = no cap.
func H264LevelMaxHeight(idc int) int {
	switch {
	case idc <= 0:
		return 0
	case idc < 31:
		return 480
	case idc < 40:
		return 720
	case idc < 50:
		return 1080
	case idc < 51:
		return 1440
	case idc < 60:
		return 2160
	}
	return 4320
}

// CapLadderToH264Level drops the rungs a decoder capped at idc cannot decode.
func CapLadderToH264Level(ladder []HLSProfile, idc int) []HLSProfile {
	return CapLadderToHeight(ladder, H264LevelMaxHeight(idc))
}

// CapLadderToVideoBitrate bounds every rung's VIDEO bitrate to ceilingKbps: a
// rung above it is clamped, and a clamped rung that no longer offers more than
// the one below it is dropped. Always keeps the lowest rung. 0 = no cap.
func CapLadderToVideoBitrate(ladder []HLSProfile, ceilingKbps int64) []HLSProfile {
	if ceilingKbps <= 0 || len(ladder) == 0 {
		return ladder
	}
	out := make([]HLSProfile, 0, len(ladder))
	for _, p := range ladder {
		if p.VideoBitrate > ceilingKbps {
			p.VideoBitrate = ceilingKbps
		}
		if len(out) > 0 && p.VideoBitrate <= out[len(out)-1].VideoBitrate {
			continue
		}
		out = append(out, p)
	}
	return out
}
