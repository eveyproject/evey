package mkv

// TrackIntegrity is one audio track of a file measured END TO END: where it is
// silent, for how long, and whether its frames all have the same shape. It is
// the unit a rewrite that touches audio is judged on, by comparison with the
// same measurement taken before - see maintenance.AudioIntegrityProber.
type TrackIntegrity struct {
	// Track is the ffmpeg audio index (0 = first audio stream).
	Track int
	Codec string
	// AudibleSec is how much of the track carries signal.
	AudibleSec float64
	// SilentStretches counts the silences of at least
	// AudioIntegritySilenceSec.
	SilentStretches int
	// FrameSizes is how many distinct packet sizes the track has. A
	// constant-frame codec (AC-3, E-AC-3, DTS) has exactly one on a sound
	// track; two means the frame format changes somewhere, which is the shape
	// that silences a player (it configures itself on the first frames and
	// gives up at the switch). 0 when the codec has variable frames and the
	// count means nothing.
	FrameSizes int
}

// AudioIntegritySilenceSec is the shortest stretch of silence that counts as
// a hole. Dialogue pauses and scene cuts sit under a second; the holes a
// broken rewrite leaves are measured in minutes.
const AudioIntegritySilenceSec = 2.0
