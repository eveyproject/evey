package ffmpeg

// Does the SOUND sit where the PICTURE is? Subtitles are timed to the picture
// and no audio repair ever touched them, so the lag between speech bursts and
// subtitle cues is the offset of that audio track - measurable without a
// reference track and without an ear. This is the method that recovered the
// 66 files of the 2026-09 campaign; on 2026-09-17 it caught a film whose two
// tracks had been pulled 0.9 s early together (nothing else could: the tracks
// agreed with each other, and the container declared no offset any more).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/pkg/procfork"
)

const (
	subSyncBinSec       = 0.02 // envelope and cue resolution
	subSyncMaxLagSec    = 45.0 // search span; dub alignments seen in the wild stay well under
	subSyncFloorDB      = -60.0
	subSyncWindowSec    = 480.0
	subSyncMinCues      = 20   // cues in a window for it to count
	subSyncMinR         = 0.15 // correlation floor for a window
	subSyncMinProm      = 1.3  // best peak over the runner-up (> 1 s away)
	subSyncAgreeSec     = 0.15 // windows agree within this
	subSyncMaxTracks    = 3
	subSyncSpeechDB     = 6.0  // a bin this far over its local background is speech
	subSyncFeatClampDB  = 12.0
	subSyncMedianHalf   = 150 // +-3 s at 20 ms
	subSyncUsableCues   = 20  // a subtitle track below this anchors nothing
	subSyncPreferCues   = 150 // enough cues: stop looking for a fuller track
	subSyncCentreProbeS = 20
)

var subSyncTimestampRe = regexp.MustCompile(`(\d+):(\d+):(\d+)[,.](\d+) --> (\d+):(\d+):(\d+)[,.](\d+)`)

type subCue struct{ start, end float64 }

// SubtitleSync measures each audio track against the file's own text
// subtitles. Needs ffprobe (streams, duration) and ffmpeg (cues, envelopes).
type SubtitleSync struct {
	ffmpegPath  string
	ffprobePath string
	scratchDir  string
}

// NewSubtitleSync builds the prober; scratchDir receives the per-track
// envelope files ffmpeg writes (one pass decodes every track).
func NewSubtitleSync(ffmpegPath, ffprobePath, scratchDir string) *SubtitleSync {
	return &SubtitleSync{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath, scratchDir: scratchDir}
}

type subSyncStream struct {
	Index     int    `json:"index"`
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Channels  int    `json:"channels"`
}

type subSyncProbe struct {
	Streams []subSyncStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeAudioPictureLag implements maintenance.AudioPictureProber. Returns one
// entry per measured audio track (ordinal), or nil when the file carries no
// text subtitle track with enough cues to anchor a measurement.
func (p *SubtitleSync) ProbeAudioPictureLag(ctx context.Context, path string) ([]appmaintenance.AudioPictureLag, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "subtitle_sync.streams"}, p.ffprobePath,
		"-v", "error", "-print_format", "json", "-show_streams", "-show_format", path)
	if err != nil {
		return nil, fmt.Errorf("subtitle sync: probe: %w", err)
	}
	var probe subSyncProbe
	if jerr := json.Unmarshal(out, &probe); jerr != nil {
		return nil, fmt.Errorf("subtitle sync: probe: %w", jerr)
	}
	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	if duration < 4*subSyncMaxLagSec {
		return nil, nil
	}
	var audios, texts []int
	for _, s := range probe.Streams {
		switch {
		case s.CodecType == "audio":
			audios = append(audios, s.Index)
		case s.CodecType == "subtitle" && isTextSubtitleCodec(s.CodecName):
			texts = append(texts, s.Index)
		}
	}
	if len(audios) == 0 || len(texts) == 0 {
		return nil, nil
	}
	cues, err := p.bestCueTrack(ctx, path, texts)
	if err != nil {
		return nil, err
	}
	if len(cues) < subSyncUsableCues {
		return nil, nil
	}
	if len(audios) > subSyncMaxTracks {
		audios = audios[:subSyncMaxTracks]
	}
	windows := subSyncWindows(duration, len(cues))
	centre := make([]bool, len(audios))
	for i := range audios {
		centre[i] = p.centreIsReal(ctx, path, i, windows[0].at)
	}
	results := make([][]subSyncWindow, len(audios))
	for _, w := range windows {
		envs, err := p.envelopes(ctx, path, len(audios), centre, w.at, w.length)
		if err != nil {
			return nil, err
		}
		for i := range audios {
			results[i] = append(results[i], correlateCues(envs[i], cues, w.at))
		}
	}
	lags := make([]appmaintenance.AudioPictureLag, 0, len(audios))
	for i := range audios {
		lag, conf, agree, ok := settleWindows(results[i])
		if !ok {
			continue
		}
		lags = append(lags, appmaintenance.AudioPictureLag{Track: i, LagSec: lag, Confidence: conf, Windows: agree})
	}
	return lags, nil
}

func isTextSubtitleCodec(codec string) bool {
	switch strings.ToLower(codec) {
	case "subrip", "srt", "ass", "ssa", "webvtt", "mov_text", "text":
		return true
	}
	return false
}

// bestCueTrack extracts the cues of the text subtitle track that carries the
// most of them: a "full" track that only holds a few forced lines cannot
// anchor anything.
func (p *SubtitleSync) bestCueTrack(ctx context.Context, path string, texts []int) ([]subCue, error) {
	var best []subCue
	for ordinal := range texts {
		cues, err := p.textCues(ctx, path, ordinal)
		if err != nil {
			return nil, err
		}
		if len(cues) > len(best) {
			best = cues
		}
		if len(best) >= subSyncPreferCues {
			break
		}
	}
	return best, nil
}

func (p *SubtitleSync) textCues(ctx context.Context, path string, ordinal int) ([]subCue, error) {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Scan, Site: "subtitle_sync.cues"}, p.ffmpegPath,
		"-v", "error", "-nostdin", "-vn", "-an", "-dn", "-i", path,
		"-map", fmt.Sprintf("0:s:%d", ordinal), "-c:s", "srt", "-f", "srt", "-")
	if err != nil {
		return nil, fmt.Errorf("subtitle sync: cues of s:%d: %w", ordinal, err)
	}
	var cues []subCue
	for _, m := range subSyncTimestampRe.FindAllStringSubmatch(string(out), -1) {
		cues = append(cues, subCue{srtSeconds(m[1:5]), srtSeconds(m[5:9])})
	}
	sort.Slice(cues, func(i, j int) bool { return cues[i].start < cues[j].start })
	return cues, nil
}

func srtSeconds(m []string) float64 {
	h, _ := strconv.Atoi(m[0])
	mi, _ := strconv.Atoi(m[1])
	s, _ := strconv.Atoi(m[2])
	frac, _ := strconv.Atoi(m[3])
	return float64(h)*3600 + float64(mi)*60 + float64(s) + float64(frac)/math.Pow(10, float64(len(m[3])))
}

type subSyncWin struct{ at, length float64 }

// subSyncWindows spreads the measurement over the film: three 8 min windows
// (two on a short one), or two long halves when the cues are sparse.
func subSyncWindows(duration float64, cueCount int) []subSyncWin {
	if cueCount < 400 {
		half := duration * 0.45
		return []subSyncWin{{duration * 0.05, half}, {duration * 0.5, half}}
	}
	fracs := []float64{0.18, 0.42, 0.66}
	length := subSyncWindowSec
	if duration < 2400 {
		fracs = []float64{0.2, 0.55}
		length = math.Min(length, duration*0.25)
	}
	var out []subSyncWin
	for _, f := range fracs {
		at := math.Max(math.Floor(duration*f-length/2), subSyncMaxLagSec)
		out = append(out, subSyncWin{at, length})
	}
	return out
}

// centreIsReal probes 20 s of the centre channel: many "5.1" dubs are a
// stereo mix with an empty centre, and the dialogue then lives in L/R.
func (p *SubtitleSync) centreIsReal(ctx context.Context, path string, ordinal int, at float64) bool {
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Scan, Site: "subtitle_sync.centre"}, p.ffmpegPath,
		"-v", "error", "-nostdin", "-vn", "-sn", "-dn",
		"-ss", strconv.FormatFloat(at, 'f', 3, 64), "-i", path, "-map", fmt.Sprintf("0:a:%d", ordinal),
		"-copyts", "-to", strconv.FormatFloat(at+subSyncCentreProbeS, 'f', 3, 64), "-af",
		"pan=mono|c0=c2,astats=metadata=1:reset=1:length=0.02,ametadata=print:key=lavfi.astats.Overall.RMS_level:file=-",
		"-f", "null", "-")
	if err != nil {
		return false
	}
	loud := 0
	for line := range strings.SplitSeq(string(out), "\n") {
		if i := strings.Index(line, "RMS_level="); i >= 0 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(line[i+10:]), 64); err == nil && v > subSyncFloorDB {
				loud++
			}
		}
	}
	return loud > 50
}

// envelopes decodes [at, at+length) of every track in ONE pass and returns the
// speech-band loudness per 20 ms bin (dB, floor when silent). Timestamps come
// from ffmpeg itself (-copyts), never from an assumed step.
func (p *SubtitleSync) envelopes(ctx context.Context, path string, tracks int, centre []bool, at, length float64) ([][]float64, error) {
	dir, err := os.MkdirTemp(p.scratchDir, "subsync-")
	if err != nil {
		return nil, fmt.Errorf("subtitle sync: scratch: %w", err)
	}
	defer os.RemoveAll(dir) // scratch dir, best-effort cleanup
	args := []string{"-v", "error", "-nostdin", "-vn", "-sn", "-dn",
		"-ss", strconv.FormatFloat(at, 'f', 3, 64), "-i", path, "-copyts"}
	for i := range tracks {
		pan := "aformat=channel_layouts=mono"
		if centre[i] {
			pan = "pan=mono|c0=c2"
		}
		filter := pan + ",highpass=f=250,lowpass=f=3500,astats=metadata=1:reset=1:length=0.02," +
			"ametadata=print:key=lavfi.astats.Overall.RMS_level:file=" + filepath.Join(dir, fmt.Sprintf("env_%d.txt", i))
		args = append(args, "-map", fmt.Sprintf("0:a:%d", i), "-to", strconv.FormatFloat(at+length, 'f', 3, 64), "-af", filter, "-f", "null", "-")
	}
	if out, err := procfork.CombinedOutput(ctx, procfork.Options{Class: procfork.Scan, Site: "subtitle_sync.envelope"}, p.ffmpegPath, args...); err != nil {
		return nil, fmt.Errorf("subtitle sync: envelope: %w: %s", err, firstLine(out))
	}
	n := int(length / subSyncBinSec)
	res := make([][]float64, tracks)
	for i := range tracks {
		env, err := readEnvelope(filepath.Join(dir, fmt.Sprintf("env_%d.txt", i)), at, n)
		if err != nil {
			return nil, err
		}
		res[i] = env
	}
	return res, nil
}

func readEnvelope(path string, at float64, n int) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("subtitle sync: envelope: %w", err)
	}
	defer f.Close()
	env := make([]float64, n)
	for j := range env {
		env[j] = subSyncFloorDB
	}
	pts := -1.0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if idx := strings.Index(line, "pts_time:"); idx >= 0 {
			if v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+9:]), 64); err == nil {
				pts = v
			}
			continue
		}
		idx := strings.Index(line, "RMS_level=")
		if idx < 0 || pts < 0 {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+10:]), 64)
		if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
			v = subSyncFloorDB
		}
		v = math.Max(v, subSyncFloorDB)
		b0 := int((pts - at) / subSyncBinSec)
		b1 := int((pts + 0.032 - at) / subSyncBinSec)
		for b := b0; b <= b1; b++ {
			if b >= 0 && b < n && v > env[b] {
				env[b] = v
			}
		}
	}
	return env, sc.Err()
}

type subSyncWindow struct {
	lag, r, prom float64
	cues         int
}

// correlateCues finds the lag (seconds, positive = the audio must move later)
// at which speech bursts best line up with the cue activity of one window.
// The feature is how far each bin rises over its local background (rolling
// median over +-3 s): dialogue is short bursts over whatever the scene's
// music sits at; raw loudness follows the music instead.
func correlateCues(env []float64, cues []subCue, at float64) subSyncWindow {
	n := len(env)
	span := int(subSyncMaxLagSec / subSyncBinSec)
	total := n + 2*span
	act := make([]float64, total)
	inWin := 0
	for _, c := range cues {
		b0 := int((c.start-at)/subSyncBinSec) + span
		b1 := int((c.end-at)/subSyncBinSec) + span
		if b1 < 0 || b0 >= total {
			continue
		}
		if c.start >= at && c.start < at+float64(n)*subSyncBinSec {
			inWin++
		}
		for b := max(b0, 0); b <= b1 && b < total; b++ {
			act[b] = 1
		}
	}
	feat := speechFeature(env)
	mean := 0.0
	for _, v := range feat {
		mean += v
	}
	mean /= float64(n)
	var sxx float64
	for i := range feat {
		feat[i] -= mean
		sxx += feat[i] * feat[i]
	}
	best, bestLag := -2.0, 0
	rs := make([]float64, 2*span+1)
	for l := -span; l <= span; l++ {
		var sxy, sy, syy float64
		for i := range n {
			y := act[i+span+l]
			sxy += feat[i] * y
			sy += y
			syy += y * y
		}
		ym := sy / float64(n)
		r := 0.0
		if den := math.Sqrt(sxx * (syy - float64(n)*ym*ym)); den > 0 {
			r = sxy / den
		}
		rs[l+span] = r
		if r > best {
			best, bestLag = r, l
		}
	}
	second := -2.0
	for l := -span; l <= span; l++ {
		if math.Abs(float64(l-bestLag))*subSyncBinSec > 1.0 && rs[l+span] > second {
			second = rs[l+span]
		}
	}
	prom := 0.0
	switch {
	case second > 0:
		prom = best / second
	case best > 0:
		prom = 99
	}
	return subSyncWindow{lag: float64(bestLag) * subSyncBinSec, r: best, prom: prom, cues: inWin}
}

func speechFeature(env []float64) []float64 {
	n := len(env)
	feat := make([]float64, n)
	buf := make([]float64, 0, 2*subSyncMedianHalf/3+2)
	for i := range env {
		buf = buf[:0]
		for j := max(0, i-subSyncMedianHalf); j <= min(n-1, i+subSyncMedianHalf); j += 3 {
			buf = append(buf, env[j])
		}
		sort.Float64s(buf)
		v := env[i] - buf[len(buf)/2]
		feat[i] = math.Min(math.Max(v, 0), subSyncFeatClampDB)
	}
	return feat
}

// settleWindows turns the per-window measurements into one lag: the median of
// the windows worth believing, accepted only when at least two of them agree.
func settleWindows(ws []subSyncWindow) (lag, confidence float64, agree int, ok bool) {
	var lags []float64
	minR := 1.0
	for _, w := range ws {
		if w.cues >= subSyncMinCues && w.r >= subSyncMinR && w.prom >= subSyncMinProm {
			lags = append(lags, w.lag)
			minR = math.Min(minR, w.r)
		}
	}
	if len(lags) == 0 {
		return 0, 0, 0, false
	}
	sort.Float64s(lags)
	lag = lags[len(lags)/2]
	for _, l := range lags {
		if math.Abs(l-lag) <= subSyncAgreeSec {
			agree++
		}
	}
	if agree < 2 {
		return 0, 0, agree, false
	}
	return lag, minR, agree, true
}
