package playback

import (
	"strconv"
	"time"
)

// What a play session shows is what happened, never what was decided. Two
// witnesses fill it, and nobody else writes it:
//
//   - the server, at the response that carried the bytes: which engine wrote
//     them, whether an encoder ran, which source track they are, and the codec
//     read from the init segment (or the file) that went out;
//   - the player, about what it decodes: the track its audio buffer holds,
//     the codec its decoder was configured with, the wasm decoder at work.
//
// The decision engine's verdict, the client's own echo of it and the plan's
// delivery strategy are inputs to serving. They are not evidence of it, and a
// runtime fallback (a packager that could not build, a wasm decoder latched
// back to AAC, a direct play the browser refused) contradicts them.

// Leg is one kind of media bytes a session receives.
type Leg string

const (
	LegVideo Leg = "video"
	LegAudio Leg = "audio"
)

// Engines that put bytes on the wire.
const (
	EngineMkvgo  = "mkvgo"  // the on-demand packager: the source's samples, repackaged
	EngineFFmpeg = "ffmpeg" // a spawned ffmpeg: a stream copy or an encode
	EngineFile   = "file"   // the source file itself, byte ranges of it (direct play)
)

// ServedLeg is what the server sent for one leg, as observed on the response
// that carried it.
type ServedLeg struct {
	// Engine wrote the bytes (EngineMkvgo, EngineFFmpeg, EngineFile).
	Engine string `json:"engine"`
	// Copy: the bytes are the source's own samples, no encoder ran. Read from
	// the producer that wrote them (mkvgo never encodes; ffmpeg's command line
	// says -c copy or not), not from a verdict.
	Copy bool `json:"copy"`
	// Track is the source's audio track (0-based, in container order) the
	// audio bytes are. -1 when the response carries every track (the file
	// itself) or when the server could not tell which one it is.
	Track int `json:"track"`
	// Codec is the codec of the bytes, read from the init segment that went
	// out (or the file's own stream on a direct play).
	Codec string `json:"codec,omitempty"`
	// SourceCodec is the source track's codec, for a leg whose bytes were
	// encoded from it.
	SourceCodec string `json:"source_codec,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Channels    int    `json:"channels,omitempty"`
	// Backend is the encoder machinery of an ffmpeg video encode ("software",
	// "qsv", "vaapi", "nvenc"); empty on a copy.
	Backend string `json:"backend,omitempty"`
	// At is when these bytes were sent.
	At time.Time `json:"at"`
}

// SameBytes reports whether two observations describe the same served leg:
// everything but the time it was seen.
func (l ServedLeg) SameBytes(o ServedLeg) bool {
	l.At, o.At = time.Time{}, time.Time{}
	return l == o
}

// Served is the latest observation of each leg. A nil leg was never served.
type Served struct {
	Video *ServedLeg `json:"video,omitempty"`
	Audio *ServedLeg `json:"audio,omitempty"`
}

// Method names what the served bytes are: the file itself is a direct play,
// a segmented stream of copied samples a remux (direct_stream), and a stream
// with any encoded leg a transcode. "" until bytes were served.
func (s Served) Method() PlayMethod {
	legs := []*ServedLeg{s.Video, s.Audio}
	seen := false
	for _, l := range legs {
		if l == nil {
			continue
		}
		seen = true
		if l.Engine == EngineFile {
			return DirectPlay
		}
	}
	if !seen {
		return ""
	}
	for _, l := range legs {
		if l != nil && !l.Copy {
			return Transcode
		}
	}
	return DirectStream
}

// Summary is the one-line form the play history and the play-method counts
// keep of a session: its method, the served codecs and the picture height
// ("1080p"). Derived from Served alone, written with it.
func (s Served) Summary() (method, videoCodec, audioCodec, resolution string) {
	method = string(s.Method())
	if s.Video != nil {
		videoCodec = s.Video.Codec
		if s.Video.Height > 0 {
			resolution = strconv.Itoa(s.Video.Height) + "p"
		}
	}
	if s.Audio != nil {
		audioCodec = s.Audio.Codec
	}
	return method, videoCodec, audioCodec, resolution
}

// Audio decoders a player reports.
const (
	DecoderNative = "native" // the platform's own decoder (MSE, AVPlayer, ExoPlayer)
	DecoderWasm   = "wasm"   // Evey's AC-3/E-AC-3 wasm decoder in the browser
)

// Sources a player reports reading from.
const (
	PlayedHLS  = "hls"  // a segmented presentation (HLS or DASH)
	PlayedFile = "file" // the source file itself
)

// Played is what the player reports it decodes, the last time it said so.
type Played struct {
	// Source is what the player reads (PlayedHLS, PlayedFile).
	Source string `json:"source"`
	// VideoCodec / Width / Height: the picture the video decoder was set up
	// for (the codec string of the buffer it appends to, the rendition it is
	// on).
	VideoCodec string `json:"video_codec,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	// AudioTrack is the source's audio track (0-based) the player plays; -1
	// when it cannot tell.
	AudioTrack int `json:"audio_track"`
	// AudioCodec is the codec its audio decoder was set up for.
	AudioCodec string `json:"audio_codec,omitempty"`
	// AudioDecoder is DecoderNative or DecoderWasm.
	AudioDecoder string    `json:"audio_decoder,omitempty"`
	At           time.Time `json:"at"`
}

// SameState reports whether two reports describe the same playback, the time
// they were made aside.
func (p Played) SameState(o Played) bool {
	p.At, o.At = time.Time{}, time.Time{}
	return p == o
}
