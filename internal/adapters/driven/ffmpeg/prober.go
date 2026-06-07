package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type Prober struct {
	binary string
}

func NewProber(binary string) *Prober {
	return &Prober{binary: binary}
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	Index         int               `json:"index"`
	CodecName     string            `json:"codec_name"`
	CodecLongName string            `json:"codec_long_name"`
	CodecType     string            `json:"codec_type"`
	Profile       string            `json:"profile"`
	Level         int               `json:"level"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	RFrameRate    string            `json:"r_frame_rate"`
	PixFmt        string            `json:"pix_fmt"`
	ColorSpace    string            `json:"color_space"`
	ColorTransfer string            `json:"color_transfer"`
	ColorPrimaries string           `json:"color_primaries"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	SampleRate    string            `json:"sample_rate"`
	BitsPerRawSample string         `json:"bits_per_raw_sample"`
	BitRate       string            `json:"bit_rate"`
	Disposition   ffprobeDisposition `json:"disposition"`
	Tags          map[string]string `json:"tags"`
}

type ffprobeDisposition struct {
	Default  int `json:"default"`
	Forced   int `json:"forced"`
}

func (p *Prober) ProbeStreams(ctx context.Context, filePath string) (*library.ProbeResult, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.prober.ProbeStreams")
	defer span.End()
	cmd := exec.CommandContext(ctx, p.binary,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("ffprobe parse: %w", err)
	}

	var streams []*library.MediaStream
	for _, s := range probe.Streams {
		ms := &library.MediaStream{
			ID:            uuid.New().String(),
			StreamIndex:   s.Index,
			StreamType:    codecTypeToStreamType(s.CodecType),
			Codec:         s.CodecName,
			CodecLongName: s.CodecLongName,
			Profile:       s.Profile,
			IsDefault:     s.Disposition.Default == 1,
			IsForced:      s.Disposition.Forced == 1,
		}
		if lang, ok := s.Tags["language"]; ok {
			ms.Language = lang
		}
		if title, ok := s.Tags["title"]; ok {
			ms.Title = title
		}
		if s.Level > 0 { // ffprobe emits -99 for "unknown"
			ms.Level = &s.Level
		}
		if s.Width > 0 {
			ms.Width = &s.Width
		}
		if s.Height > 0 {
			ms.Height = &s.Height
		}
		if fps := parseFrameRate(s.RFrameRate); fps > 0 {
			ms.Framerate = &fps
		}
		if s.ColorSpace != "" {
			cs := s.ColorSpace
			ms.ColorSpace = &cs
		}
		if s.Channels > 0 {
			ms.Channels = &s.Channels
		}
		if s.ChannelLayout != "" {
			cl := s.ChannelLayout
			ms.ChannelLayout = &cl
		}
		if sr, err := strconv.Atoi(s.SampleRate); err == nil && sr > 0 {
			ms.SampleRate = &sr
		}
		if bd, err := strconv.Atoi(s.BitsPerRawSample); err == nil && bd > 0 {
			ms.BitDepth = &bd
		}
		if br, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil && br > 0 {
			ms.Bitrate = &br
		}
		streams = append(streams, ms)
	}

	var duration shared.Duration
	if dur, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && dur > 0 {
		// Ignore conversion errors here: an invalid duration just means we
		// can't report one — the streams are still valid and the scan/probe
		// chain treats a zero duration as "unknown" and recovers on the next
		// pass once the file has the right metadata.
		if d, derr := shared.NewDurationMs(int64(dur * 1000)); derr == nil {
			duration = d
		}
	}

	return &library.ProbeResult{Streams: streams, Duration: duration}, nil
}

// ProbeKeyframes returns the presentation timestamps (milliseconds, sorted
// ascending, deduped) of the first video stream's keyframes. These are the ONLY
// valid segment-start boundaries for a -c:v copy (remux) rendition: a copied
// segment cannot begin mid-GOP, so a copy rung's segments must align to the
// source's existing keyframes. Reads packet headers only (no decode).
//
// WARNING — cost: on a Matroska source this scans every packet header (~40 s for
// a 2 h film, no random access), so it is for a PACED background backfill only,
// never a request hot path. The fast path is the container's own seek index
// (MKV Cues / MP4 stss); see the mkvgo OpenCues enhancement note in memory. This
// ffprobe scan is the correct-but-slow fallback.
func (p *Prober) ProbeKeyframes(ctx context.Context, filePath string) ([]int64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.prober.ProbeKeyframes")
	defer span.End()
	cmd := exec.CommandContext(ctx, p.binary,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags",
		"-of", "csv=p=0",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe keyframes: %w", err)
	}
	return parseKeyframeCSV(out), nil
}

// parseKeyframeCSV extracts keyframe presentation times (ms, sorted, deduped)
// from `ffprobe -show_entries packet=pts_time,flags -of csv=p=0` output. Each
// line is "<pts_time_seconds>,<flags>"; a keyframe packet carries 'K' as the
// first flag char. pts_time is emitted in DECODE order (B-frames make it
// non-monotonic), so the result is sorted before return.
func parseKeyframeCSV(out []byte) []int64 {
	lines := strings.Split(string(out), "\n")
	seen := make(map[int64]struct{}, len(lines))
	kf := make([]int64, 0, len(lines)/8)
	for _, line := range lines {
		comma := strings.IndexByte(line, ',')
		if comma < 0 || comma+1 >= len(line) || line[comma+1] != 'K' {
			continue
		}
		sec, err := strconv.ParseFloat(line[:comma], 64)
		if err != nil || sec < 0 {
			continue
		}
		ms := int64(sec*1000 + 0.5)
		if _, dup := seen[ms]; dup {
			continue
		}
		seen[ms] = struct{}{}
		kf = append(kf, ms)
	}
	slices.Sort(kf)
	return kf
}

func codecTypeToStreamType(t string) library.StreamType {
	switch t {
	case "video":
		return library.StreamTypeVideo
	case "audio":
		return library.StreamTypeAudio
	case "subtitle":
		return library.StreamTypeSubtitle
	default:
		return library.StreamTypeAttachment
	}
}

func parseFrameRate(r string) float64 {
	parts := strings.SplitN(r, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}
