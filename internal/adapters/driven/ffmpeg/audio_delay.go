package ffmpeg

// Source audio start-delay probe: how far the selected audio track's first
// timestamp sits AFTER the video's. Some repacks carry ~1s of it as a mux
// defect; the HLS pipeline preserves source PTS, so without cancellation the
// viewer hears a constant A/V offset (see appendAudioDelayInput). The probe
// runs at session start (HLSTranscoder.Start) — a header-only ffprobe — and
// caches per file identity so seek-spawned sessions don't re-pay it.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// audioDelayProbeBinary mirrors seekLandingBinary: the bundled ffprobe.
const audioDelayProbeBinary = "ffprobe"

// audioDelayCache memoizes probed delays keyed by file identity + track. One
// small entry per (file, track) ever played this process lifetime; a rewritten
// file (new size/mtime) gets a fresh key and the stale entry just idles.
var audioDelayCache sync.Map

// audioDelayCacheLen approximates the entry count so the cache stays bounded:
// past the cap the whole map is dropped (entries re-derive lazily) - simpler
// and safer than LRU bookkeeping for values this cheap to recompute.
var audioDelayCacheLen atomic.Int64

const audioDelayCacheCap = 8192

// audioStartDelayFor returns the delay BuildFFmpegArgs should cancel for this
// session, or 0 (strict no-op) when it does not apply: no audio, copied audio
// (never delay-cancelled — see audioDelayCancelActive), probe failure (fail
// soft: the session plays with the source's delay, exactly the pre-fix
// behaviour), or a delay under the cancel floor.
func audioStartDelayFor(ctx context.Context, opts transcoding.HLSProcessOpts) float64 {
	if !hasAudio(opts) || opts.AudioDecision != "transcode" {
		return 0
	}
	fi, err := os.Stat(opts.FilePath)
	if err != nil {
		return 0
	}
	key := fmt.Sprintf("%s|%d|%d|%d", opts.FilePath, opts.AudioTrackIndex, fi.Size(), fi.ModTime().UnixNano())
	if v, ok := audioDelayCache.Load(key); ok {
		return v.(float64)
	}
	d, err := probeAudioStartDelaySec(ctx, audioDelayProbeBinary, opts.FilePath, opts.AudioTrackIndex)
	if err != nil {
		// Not cached: a transient failure (context cancel, IO hiccup) must not
		// pin "no delay" for the file's whole process lifetime.
		slog.Warn("hls: audio start-delay probe failed, session keeps the source delay",
			slog.String("media_path", opts.FilePath), slog.Any("err", err))
		return 0
	}
	if d < 0 {
		// Audio STARTING BEFORE video is a different (rarer) defect; shifting it
		// forward is untested territory — keep the no-op.
		d = 0
	}
	if d > audioDelayCancelMinSec {
		slog.Info("hls: source audio start delay detected, cancelling via shifted second input",
			slog.String("media_path", opts.FilePath),
			slog.Int("audio_track", opts.AudioTrackIndex),
			slog.Float64("delay_sec", d))
	}
	if audioDelayCacheLen.Add(1) > audioDelayCacheCap {
		audioDelayCache.Clear()
		audioDelayCacheLen.Store(1)
	}
	audioDelayCache.Store(key, d)
	return d
}

// audioDelayProbeStream is the minimal ffprobe JSON the delay probe reads.
type audioDelayProbeStream struct {
	CodecType   string `json:"codec_type"`
	StartTime   string `json:"start_time"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}

// probeAudioStartDelaySec measures start_time(audio track audioIdx) −
// start_time(first real video track) via a header-only ffprobe. audioIdx
// counts audio streams (the 0:a:N space), matching HLSProcessOpts.
// AudioTrackIndex. Cover-art streams (attached_pic) are skipped like the main
// prober does. A missing/unparseable start_time on either side yields 0, nil:
// no evidence of a delay is not an error, it is "nothing to cancel".
func probeAudioStartDelaySec(ctx context.Context, binary, filePath string, audioIdx int) (float64, error) {
	out, err := runAudioDelayProbe(ctx, binary, filePath)
	if err != nil {
		return 0, err
	}
	return parseAudioStartDelay(out, audioIdx)
}

// ProbeMaxAudioStartDelaySec is the library-diagnosis flavour of the probe:
// the LARGEST start delay across the file's audio tracks (a file is defective
// if ANY track is late). Consumed by the admin seek-index scan through the
// maintenance.AudioDelayProber port. 0 when the file has no measurable
// video+audio start_time pair.
func (p *Prober) ProbeMaxAudioStartDelaySec(ctx context.Context, filePath string) (float64, error) {
	out, err := runAudioDelayProbe(ctx, p.binary, filePath)
	if err != nil {
		return 0, err
	}
	return parseMaxAudioStartDelay(out)
}

// audioDelayProbeTimeout bounds the ffprobe run: the library can sit on a
// network mount, and a probe hung on a dead share must not wedge the caller -
// the seek-index scan holds the job-subsystem lock for its whole run, so an
// unbounded probe would block every future maintenance job until restart.
const audioDelayProbeTimeout = 15 * time.Second

func runAudioDelayProbe(ctx context.Context, binary, filePath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, audioDelayProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary,
		"-v", "error",
		"-show_entries", "stream=codec_type,start_time:stream_disposition=attached_pic",
		"-of", "json",
		filePath,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe audio-delay: %w", err)
	}
	return out, nil
}

func parseAudioStartDelay(out []byte, audioIdx int) (float64, error) {
	var probe struct {
		Streams []audioDelayProbeStream `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, fmt.Errorf("ffprobe audio-delay parse: %w", err)
	}
	videoStart, videoOK := 0.0, false
	audioStart, audioOK := 0.0, false
	nAudio := 0
	for _, s := range probe.Streams {
		switch s.CodecType {
		case "video":
			if !videoOK && s.Disposition.AttachedPic == 0 {
				videoStart, videoOK = parseProbeSeconds(s.StartTime)
			}
		case "audio":
			if nAudio == audioIdx {
				audioStart, audioOK = parseProbeSeconds(s.StartTime)
			}
			nAudio++
		}
	}
	if !videoOK || !audioOK {
		return 0, nil
	}
	return audioStart - videoStart, nil
}

func parseMaxAudioStartDelay(out []byte) (float64, error) {
	var probe struct {
		Streams []audioDelayProbeStream `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, fmt.Errorf("ffprobe audio-delay parse: %w", err)
	}
	videoStart, videoOK := 0.0, false
	maxDelay := 0.0
	for _, s := range probe.Streams {
		if s.CodecType == "video" && !videoOK && s.Disposition.AttachedPic == 0 {
			videoStart, videoOK = parseProbeSeconds(s.StartTime)
		}
	}
	if !videoOK {
		return 0, nil
	}
	for _, s := range probe.Streams {
		if s.CodecType != "audio" {
			continue
		}
		if start, ok := parseProbeSeconds(s.StartTime); ok {
			if d := start - videoStart; d > maxDelay {
				maxDelay = d
			}
		}
	}
	return maxDelay, nil
}

// parseProbeSeconds parses ffprobe's start_time ("0.900000", "N/A" or empty).
func parseProbeSeconds(s string) (float64, bool) {
	if s == "" || s == "N/A" {
		return 0, false
	}
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return 0, false
	}
	return v, true
}
