package transcoding

type ClientProfile struct {
	VideoCodecs      []string // e.g. ["h264", "vp9", "av1"]
	AudioCodecs      []string // e.g. ["aac", "mp3", "opus", "flac"]
	Containers       []string // e.g. ["mp4", "webm", "mkv"]
	MaxWidth         int
	MaxHeight        int
	MaxBitrate       int64 // kbps, 0 = unlimited
	SupportsHDR      bool
	MaxAudioChannels int // 0 = unlimited
	// MaxH264Level is the highest H.264 level_idc the device decodes (41 = 4.1,
	// a first-generation Chromecast). 0 = no limit.
	MaxH264Level int
	// FMP4AudioPassthrough lists the audio codecs this client accepts as a RAW
	// passthrough inside our fMP4 segments, on top of the always-wired set
	// (AAC/AC-3/E-AC-3). Deliberately NOT derived from AudioCodecs: "I decode
	// MP3" and "I accept MP3 muxed in fMP4" are different questions, and the
	// web player declares mp3 unconditionally (utils/codec.ts) while AVPlayer
	// refuses it in that container. Empty (the default) keeps every client that
	// says nothing on the historical behaviour: re-encode to AAC.
	FMP4AudioPassthrough []string
}

type PlaybackDecision struct {
	PlayMethod       string   `json:"play_method"`
	VideoDecision    string   `json:"video_decision"`
	AudioDecision    string   `json:"audio_decision"`
	SubtitleDecision string   `json:"subtitle_decision"`
	TranscodeReasons []string `json:"transcode_reasons"`
	VideoCodec       string   `json:"video_codec,omitempty"`
	// ClientVideoCodecs: what the client decodes, so the resolver can pick HEVC output when the host encodes it.
	ClientVideoCodecs []string `json:"client_video_codecs,omitempty"`
	AudioCodec        string   `json:"audio_codec,omitempty"`
	Container         string   `json:"container,omitempty"`
	VideoBitrate      int64    `json:"video_bitrate,omitempty"`
	AudioBitrate      int64    `json:"audio_bitrate,omitempty"`
	MaxWidth          int      `json:"max_width,omitempty"`
	MaxHeight         int      `json:"max_height,omitempty"`
	MaxAudioChannels  int      `json:"max_audio_channels,omitempty"`
}

var DefaultWebProfile = ClientProfile{
	VideoCodecs: []string{"h264", "vp9"},
	AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	Containers:  []string{"mp4", "webm"},
	MaxWidth:    3840,
	MaxHeight:   2160,
	SupportsHDR: false,
	// 6 (5.1), not 8: no browser decodes AAC 7.1 in MSE - Chrome tops out at 5.1.
	// Claiming 8 makes the pipeline emit `-ac 8`, the SourceBuffer refuses the
	// stream, and the title never starts (seen on a real Dolby Atmos 7.1 source:
	// the audio came out `aac 8ch 7.1 @422k` and playback never began). A client
	// that genuinely decodes more declares it itself (MediaCaps); this is only the
	// default for one that declares nothing.
	MaxAudioChannels: 6,
}
