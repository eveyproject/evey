package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/eveyproject/evey/internal/pkg/procfork"

	"github.com/google/uuid"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type Prober struct {
	binary string
}

func NewProber(binary string) *Prober {
	return &Prober{binary: binary}
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	// FormatName is ffprobe's (comma-joined) container list, e.g.
	// "matroska,webm" or "mov,mp4,m4a,3gp,3g2,mj2". normalizeContainer collapses
	// it to the single name the playback decision matches on.
	FormatName string `json:"format_name"`
}

// normalizeContainer collapses ffprobe's comma-joined format_name to one
// container name in the decision's vocabulary. "matroska"/"webm" -> "matroska"
// (checkContainer maps it to "mkv"; WebM is a Matroska subset), the MP4 family
// -> "mp4"; anything else keeps its first token (avi, mpegts, flv, ...).
func normalizeContainer(formatName string) string {
	f := strings.ToLower(strings.TrimSpace(formatName))
	switch {
	case f == "":
		return ""
	case strings.Contains(f, "matroska"), strings.Contains(f, "webm"):
		return "matroska"
	case strings.Contains(f, "mp4"), strings.Contains(f, "mov"), strings.Contains(f, "m4a"), strings.Contains(f, "3gp"):
		return "mp4"
	default:
		if i := strings.IndexByte(f, ','); i > 0 {
			return f[:i]
		}
		return f
	}
}

type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	Index             int                `json:"index"`
	CodecName         string             `json:"codec_name"`
	CodecLongName     string             `json:"codec_long_name"`
	CodecType         string             `json:"codec_type"`
	Profile           string             `json:"profile"`
	Level             int                `json:"level"`
	Width             int                `json:"width"`
	Height            int                `json:"height"`
	RFrameRate        string             `json:"r_frame_rate"`
	PixFmt            string             `json:"pix_fmt"`
	FieldOrder        string             `json:"field_order"`
	SampleAspectRatio string             `json:"sample_aspect_ratio"`
	ColorSpace        string             `json:"color_space"`
	ColorTransfer     string             `json:"color_transfer"`
	ColorPrimaries    string             `json:"color_primaries"`
	Channels          int                `json:"channels"`
	ChannelLayout     string             `json:"channel_layout"`
	SampleRate        string             `json:"sample_rate"`
	BitsPerRawSample  string             `json:"bits_per_raw_sample"`
	BitRate           string             `json:"bit_rate"`
	Disposition       ffprobeDisposition `json:"disposition"`
	Tags              map[string]string  `json:"tags"`
	SideDataList      []ffprobeSideData  `json:"side_data_list"`
}

// ffprobeSideData carries the per-stream side data entries. The only one the
// probe consumes is the "DOVI configuration record" (Dolby Vision): dv_profile
// + dv_bl_signal_compatibility_id are the ground truth the playback decision
// needs to keep profile-5 DoVi off the raw-serve paths (its IPTC2 base layer
// renders green/purple on any non-DoVi pipeline) while letting profile 8 be
// served as its compatible base layer.
type ffprobeSideData struct {
	SideDataType       string `json:"side_data_type"`
	DVProfile          *int   `json:"dv_profile"`
	DVBLSignalCompatID *int   `json:"dv_bl_signal_compatibility_id"`
}

type ffprobeDisposition struct {
	Default         int `json:"default"`
	Forced          int `json:"forced"`
	AttachedPic     int `json:"attached_pic"`
	Comment         int `json:"comment"`
	HearingImpaired int `json:"hearing_impaired"`
	VisualImpaired  int `json:"visual_impaired"`
	Descriptions    int `json:"descriptions"`
	Original        int `json:"original"`
}

// bpsTagBitrate returns the per-track bitrate from a Matroska "BPS" tag (the
// value ffprobe surfaces only as TAG:BPS, never as bit_rate for MKV), or 0 when
// absent. mkvgo's WithBitrate reads the same "BPS" SimpleTag, so both prober
// paths report the same MKV bitrate. ffmpeg may suffix the key with a language
// (e.g. "BPS-eng"), so accept that form too.
func bpsTagBitrate(tags map[string]string) int64 {
	for k, v := range tags {
		if k == "BPS" || strings.HasPrefix(k, "BPS-") {
			if b, err := strconv.ParseInt(v, 10, 64); err == nil && b > 0 {
				return b
			}
		}
	}
	return 0
}

func (p *Prober) ProbeStreams(ctx context.Context, filePath string) (*library.ProbeResult, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.prober.ProbeStreams")
	defer span.End()
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "prober.streams"}, p.binary,
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, fmt.Errorf("ffprobe parse: %w", err)
	}

	var streams []*library.MediaStream
	hasRealVideo := false
	for _, s := range probe.Streams {
		if s.CodecType == "video" && !library.IsStillImageCodec(s.CodecName) {
			hasRealVideo = true
		}
	}
	for _, s := range probe.Streams {
		// Cover art / thumbnails ride as attached_pic "video" streams (mjpeg/png)
		// in both MP4 and Matroska; they are not playable video and the mkvgo
		// fast path does not surface them as tracks, so skip them to keep stream
		// counts and the playback decision aligned between the two probers.
		// A Matroska PNG cover muxed as a plain video track carries no
		// attached_pic flag at all: a still-image codec next to a real video
		// stream is cover art just the same.
		if s.Disposition.AttachedPic == 1 {
			continue
		}
		if s.CodecType == "video" && hasRealVideo && library.IsStillImageCodec(s.CodecName) {
			continue
		}
		ms := &library.MediaStream{
			ID:               uuid.New().String(),
			StreamIndex:      s.Index,
			StreamType:       codecTypeToStreamType(s.CodecType),
			Codec:            s.CodecName,
			CodecLongName:    s.CodecLongName,
			Profile:          s.Profile,
			IsDefault:        s.Disposition.Default == 1,
			IsForced:         s.Disposition.Forced == 1,
			Commentary:       s.Disposition.Comment == 1,
			HearingImpaired:  s.Disposition.HearingImpaired == 1,
			VisualImpaired:   s.Disposition.VisualImpaired == 1,
			TextDescriptions: s.Disposition.Descriptions == 1,
			Original:         s.Disposition.Original == 1,
		}
		// Matroska carries language in the track header (ffprobe lowercases it to
		// "language") OR, when it only lives in a Tags SimpleTag, ffprobe preserves
		// the verbatim "LANGUAGE" key - accept either so a Tags-only language is
		// not silently dropped.
		if lang, ok := s.Tags["language"]; ok {
			ms.Language = lang
		} else if lang, ok := s.Tags["LANGUAGE"]; ok {
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
		if s.PixFmt != "" {
			pf := s.PixFmt
			ms.PixelFormat = &pf
		}
		if s.ColorSpace != "" {
			cs := s.ColorSpace
			ms.ColorSpace = &cs
		}
		if s.ColorTransfer != "" {
			ct := s.ColorTransfer
			ms.ColorTransfer = &ct
		}
		ms.FieldOrder = library.NormalizeFieldOrder(s.FieldOrder)
		// "0:1" is ffprobe's "unset" sentinel, not a real sample aspect ratio.
		if s.SampleAspectRatio != "" && s.SampleAspectRatio != "0:1" {
			sar := s.SampleAspectRatio
			ms.SampleAspectRatio = &sar
		}
		for _, sd := range s.SideDataList {
			if sd.SideDataType != "DOVI configuration record" || sd.DVProfile == nil {
				continue
			}
			ms.DoViProfile = sd.DVProfile
			ms.DoViBLCompat = sd.DVBLSignalCompatID
			break
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
		} else if bps := bpsTagBitrate(s.Tags); bps > 0 {
			// Matroska keeps the per-track bitrate in a TAG:BPS, not the bit_rate
			// field (which ffprobe leaves N/A for MKV). Mirror mkvgo's WithBitrate so
			// the ffprobe fallback reports the same MKV bitrate as the fast path.
			ms.Bitrate = &bps
		}
		streams = append(streams, ms)
	}

	var duration shared.Duration
	if dur, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && dur > 0 {
		// Ignore conversion errors here: an invalid duration just means we
		// can't report one - the streams are still valid and the scan/probe
		// chain treats a zero duration as "unknown" and recovers on the next
		// pass once the file has the right metadata.
		if d, derr := shared.NewDurationMs(int64(dur * 1000)); derr == nil {
			duration = d
		}
	}

	return &library.ProbeResult{Streams: streams, Duration: duration, Container: normalizeContainer(probe.Format.FormatName)}, nil
}

// ProbeKeyframes returns the presentation timestamps (milliseconds, sorted
// ascending, deduped) of the first video stream's keyframes. These are the ONLY
// valid segment-start boundaries for a -c:v copy (remux) rendition: a copied
// segment cannot begin mid-GOP, so a copy rung's segments must align to the
// source's existing keyframes. Reads packet headers only (no decode).
//
// WARNING - cost: on a Matroska source this scans every packet header (~40 s for
// a 2 h film, no random access), so it is for a PACED background backfill only,
// never a request hot path. The fast path is the container's own seek index
// (MKV Cues / MP4 stss); see the mkvgo OpenCues enhancement note in memory. This
// ffprobe scan is the correct-but-slow fallback.
func (p *Prober) ProbeKeyframes(ctx context.Context, filePath string) ([]int64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.prober.ProbeKeyframes")
	defer span.End()
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "prober.keyframes"}, p.binary,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags",
		"-of", "csv=p=0",
		filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("ffprobe keyframes: %w", err)
	}
	return parseKeyframeCSV(out), nil
}

// ProbeInterlaceDetect runs ffmpeg's idet filter over a ~300-frame window a
// minute into the file (past studio logos / black intros) and reports whether
// the CONTENT is interlaced. This is the fallback for sources whose container
// flags are absent or lie - repacked DVD rips store interlaced fields inside
// "progressive" frames, so the decoder never tags them and the flag-gated
// bwdif (deint=interlaced) leaves the combing untouched. Cost: a ~12s decode
// of SD content (~1-2 s wall) - background backfill only, never a hot path.
//
// Verdict: interlaced when the (TFF+BFF) multi-frame counts MAJORITY the
// determined frames (more interlaced than progressive). A simple majority vote
// is conservative on purpose - synthetic and lightly-noisy progressive content
// scatters a minority of frames into TFF/BFF, so a low percentage threshold
// would false-positive and softly deinterlace progressive titles; when in
// doubt, don't. Telecined / genuinely interlaced sources clear the majority
// easily.
func (p *Prober) ProbeInterlaceDetect(ctx context.Context, filePath string) (bool, error) {
	v, err := p.ProbeIdetSummary(ctx, filePath)
	return v.Interlaced, err
}

// ProbeIdetSummary is ProbeInterlaceDetect with the telecine verdict kept.
func (p *Prober) ProbeIdetSummary(ctx context.Context, filePath string) (IdetSummary, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.prober.ProbeInterlaceDetect")
	defer span.End()
	v, ok, err := p.idetRun(ctx, filePath, true)
	if err == nil && ok {
		return v, nil
	}
	v, ok, err = p.idetRun(ctx, filePath, false)
	if err != nil {
		return IdetSummary{}, fmt.Errorf("ffmpeg idet probe: %w", err)
	}
	if !ok {
		return IdetSummary{}, fmt.Errorf("ffmpeg idet probe: zero determined frames")
	}
	return v, nil
}

// idetRun executes one idet pass and parses the verdict. ok=false means the
// run determined no frames (the caller retries without the seek). seek inserts
// the -ss 60 skip.
func (p *Prober) idetRun(ctx context.Context, filePath string, seek bool) (v IdetSummary, ok bool, err error) {
	args := []string{"-v", "info"}
	if seek {
		args = append(args, "-ss", "60")
	}
	args = append(args, "-i", filePath, "-map", "0:v:0",
		"-vf", "idet", "-frames:v", "300", "-an", "-sn", "-f", "null", "-")
	// idet prints its summary on stderr at stream close.
	out, runErr := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Scan, Site: "prober.idet"}, seekLandingBinary, args...)
	if runErr != nil {
		return IdetSummary{}, false, runErr
	}
	return parseIdetVerdict(out)
}

// parseIdetSummary extracts the multi-frame detection counters from idet's
// stderr summary:
//
//	[Parsed_idet_0 @ ...] Multi frame detection: TFF:   120 BFF:     0 Progressive:   170 Undetermined:    10
//
// Returns ok=false when no frames were determined (e.g. a seek past EOF).
// Compiled once, not per call: parseIdetSummary runs after every idet probe.
var idetSummaryRe = regexp.MustCompile(`Multi frame detection: TFF:\s*(\d+)\s*BFF:\s*(\d+)\s*Progressive:\s*(\d+)`)
var idetRepeatRe = regexp.MustCompile(`Repeated Fields: Neither:\s*(\d+)\s*Top:\s*(\d+)\s*Bottom:\s*(\d+)`)

// IdetSummary is one idet pass: Interlaced when combed frames outnumber
// progressive ones, Telecine when on top of that a good share of frames repeat
// a field (3:2 pulldown repeats one field in five).
type IdetSummary struct {
	Interlaced bool
	Telecine   bool
}

// telecineRepeatShare: 3:2 pulldown repeats a field on 2 frames in 5 (40 %);
// clean interlaced and progressive material sit near 0.
const telecineRepeatShare = 0.15

func parseIdetSummary(out []byte) (interlaced, ok bool, err error) {
	v, ok, err := parseIdetVerdict(out)
	return v.Interlaced, ok, err
}

// The LAST summary is the cumulative one (ffmpeg prints an all-zero one first).
func parseIdetVerdict(out []byte) (v IdetSummary, ok bool, err error) {
	all := idetSummaryRe.FindAllSubmatch(out, -1)
	if len(all) == 0 {
		return v, false, fmt.Errorf("no multi-frame summary in output")
	}
	m := all[len(all)-1]
	tff, _ := strconv.Atoi(string(m[1]))
	bff, _ := strconv.Atoi(string(m[2]))
	prog, _ := strconv.Atoi(string(m[3]))
	if tff+bff+prog == 0 {
		return v, false, nil
	}
	v.Interlaced = tff+bff > prog
	if rep := idetRepeatRe.FindAllSubmatch(out, -1); len(rep) > 0 && v.Interlaced {
		rm := rep[len(rep)-1]
		neither, _ := strconv.Atoi(string(rm[1]))
		top, _ := strconv.Atoi(string(rm[2]))
		bottom, _ := strconv.Atoi(string(rm[3]))
		if total := neither + top + bottom; total > 0 && float64(top+bottom)/float64(total) >= telecineRepeatShare {
			v.Telecine = true
		}
	}
	return v, true, nil
}

// seekLandingBinary is the ffmpeg CLI used by ProbeSeekLanding. It MUST be the
// same binary family the copy sessions run, because the probe's whole point is
// to reproduce the ffmpeg CLI's seek decision bit-for-bit (see below).
const seekLandingBinary = "ffmpeg"

// ProbeSeekLanding returns the presentation time (seconds) of the packet
// `ffmpeg -ss targetSec -i file -c:v copy` ACTUALLY starts on.
//
// Two effects make the landing diverge from the requested keyframe:
//   - SPARSE seek indexes: mkvmerge-style MKVs cue per cluster, not per
//     keyframe, so a backward seek can land a GOP (or several) earlier.
//   - The ffmpeg CLI's DTS heuristic: when any video stream has B-frames
//     (video_delay > 0) and the format doesn't seek by PTS, ffmpeg_demux
//     subtracts 3/23 s (~130 ms) from the seek timestamp before calling
//     avformat_seek_file - so a target sitting ON a cue point selects the
//     PREVIOUS cue. Measured: ffprobe -read_intervals (no heuristic) reported
//     1351.308 while the real session started at 1350.516 on the same target.
//
// Replicating those rules is fragile, so the probe IS ffmpeg: same binary,
// same -ss code path, `-c:v copy -frames:v 1 -f framecrc` to print the first
// copied packet (its pts kept absolute by -copyts) and exit. A copy session
// that assumes it started at the requested keyframe mis-labels every video
// timestamp by the landing gap (measured in prod: -ss 1201.250 landed at
// 1194.861 → +6.34 s of A/V desync after a deep seek). Cheap: header + seek
// index + one packet, no decode.
func (p *Prober) ProbeSeekLanding(ctx context.Context, filePath string, targetSec float64) (float64, error) {
	ctx, span := tracing.StartSpan(ctx, "ffmpeg.prober.ProbeSeekLanding")
	defer span.End()
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "prober.seek_landing"}, seekLandingBinary,
		"-v", "error",
		// The target is a source timestamp, exactly as the copy session's -ss.
		"-seek_timestamp", "1",
		"-ss", fmt.Sprintf("%.3f", targetSec),
		"-copyts",
		"-i", filePath,
		"-map", "0:v:0",
		"-c:v", "copy",
		"-frames:v", "1",
		"-f", "framecrc", "-",
	)
	if err != nil {
		return 0, fmt.Errorf("ffmpeg seek-landing probe: %w", err)
	}
	return parseFramecrcFirstPTS(out)
}

// parseFramecrcFirstPTS extracts the first packet's pts (seconds) from
// `-f framecrc` output:
//
//	#tb 0: 1/1000
//	0,       1350433,      1350516,       42,   123456, 0xabcd1234
//
// (fields: stream, dts, pts, duration, size, crc; pts is in the #tb timebase).
func parseFramecrcFirstPTS(out []byte) (float64, error) {
	tbNum, tbDen := int64(1), int64(1000) // matroska default; overridden by #tb
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if frac, ok := strings.CutPrefix(line, "#tb 0:"); ok {
			frac = strings.TrimSpace(frac)
			if n, d, ok := strings.Cut(frac, "/"); ok {
				num, e1 := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
				den, e2 := strconv.ParseInt(strings.TrimSpace(d), 10, 64)
				if e1 == nil && e2 == nil && den > 0 {
					tbNum, tbDen = num, den
				}
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}
		pts, perr := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
		if perr != nil {
			return 0, fmt.Errorf("ffmpeg seek-landing probe: unparseable pts in %q", line)
		}
		sec := float64(pts) * float64(tbNum) / float64(tbDen)
		if sec < 0 {
			return 0, fmt.Errorf("ffmpeg seek-landing probe: negative pts %d", pts)
		}
		return sec, nil
	}
	return 0, fmt.Errorf("ffmpeg seek-landing probe: no packet line in framecrc output")
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

// ProbeContainerDurationSec returns the container duration in seconds via
// ffprobe. A thin exported helper for the admin probe CLI.
func ProbeContainerDurationSec(ctx context.Context, ffprobeBin, path string) (float64, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "prober.duration"}, ffprobeBin,
		"-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}
