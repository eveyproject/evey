package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

var reProgress = regexp.MustCompile(
	`frame=\s*(\d+)\s+fps=\s*([\d.]+)\s+.*?size=\s*\S+\s+time=\s*(\d+):(\d+):(\d+)\.(\d+)\s+bitrate=\s*([\d.]+)kbits/s\s+speed=\s*([\d.]+)x`,
)

type runningJob struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	progress *transcoding.TranscodeProgress
	mu       sync.Mutex
}

type Transcoder struct {
	binaryPath string
	outputDir  string
	jobs       sync.Map
}

func NewTranscoder(binaryPath, outputDir string) *Transcoder {
	return &Transcoder{binaryPath: binaryPath, outputDir: outputDir}
}

func (t *Transcoder) Start(ctx context.Context, job *transcoding.TranscodeJob) (string, error) {
	args := t.BuildTranscodeArgs(job)

	jobCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(jobCtx, t.binaryPath, args...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("start ffmpeg: %w", err)
	}

	rj := &runningJob{
		cmd:    cmd,
		cancel: cancel,
		progress: &transcoding.TranscodeProgress{
			JobID: job.ID,
		},
	}
	t.jobs.Store(job.ID, rj)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("transcoder: panic in stderr reader", slog.Any("panic", r), slog.String("jobID", job.ID))
			}
			if err := cmd.Wait(); err != nil {
				slog.Warn("transcoder wait", slog.String("jobID", job.ID), slog.Any("err", err))
			}
			t.jobs.Delete(job.ID)
		}()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		scanner.Split(scanFFmpegLines)
		for scanner.Scan() {
			if jobCtx.Err() != nil {
				return
			}
			line := scanner.Text()
			if p, ok := ParseProgress(line, job.ID, job.DurationMs); ok {
				rj.mu.Lock()
				rj.progress = p
				rj.mu.Unlock()
			}
		}
	}()

	return job.ID, nil
}

func (t *Transcoder) Stop(_ context.Context, jobID string) error {
	v, ok := t.jobs.Load(jobID)
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	rj, ok2 := v.(*runningJob)
	if !ok2 {
		return fmt.Errorf("job %s: invalid type", jobID)
	}
	rj.cancel()
	t.jobs.Delete(jobID)
	return nil
}

func (t *Transcoder) Progress(_ context.Context, jobID string) (*transcoding.TranscodeProgress, error) {
	v, ok := t.jobs.Load(jobID)
	if !ok {
		return nil, fmt.Errorf("job %s not found", jobID)
	}
	rj, ok2 := v.(*runningJob)
	if !ok2 {
		return nil, fmt.Errorf("job %s: invalid type", jobID)
	}
	rj.mu.Lock()
	defer rj.mu.Unlock()
	p := *rj.progress
	return &p, nil
}

func (t *Transcoder) BuildTranscodeArgs(job *transcoding.TranscodeJob) []string {
	args := []string{"-y", "-progress", "pipe:2"}

	hwAccel := transcoding.HWAccelType(job.HWAccelType)
	if job.HWAccelType != "" && hwAccel != transcoding.None {
		switch hwAccel {
		case transcoding.VAAPI:
			args = append(args, "-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi")
			if _, err := os.Stat("/dev/dri/renderD128"); err == nil {
				args = append(args, "-vaapi_device", "/dev/dri/renderD128")
			}
		case transcoding.NVENC:
			args = append(args, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
		case transcoding.QSV:
			args = append(args, "-hwaccel", "qsv", "-hwaccel_output_format", "qsv")
		}
	}

	args = append(args, "-i", job.InputPath)

	vcodec := mapVideoEncoder(job.OutputCodec, job.HWAccelType)
	args = append(args, "-c:v", vcodec)

	if job.Preset != "" {
		args = append(args, "-preset", job.Preset)
	}
	if job.BitrateOut > 0 {
		args = append(args, "-b:v", strconv.FormatInt(job.BitrateOut, 10))
	}
	if job.ResolutionOut != "" {
		var scaleFilter string
		switch hwAccel {
		case transcoding.VAAPI:
			scaleFilter = "scale_vaapi=w=" + strings.Replace(job.ResolutionOut, ":", ":h=", 1)
		case transcoding.NVENC:
			scaleFilter = "scale_cuda=w=" + strings.Replace(job.ResolutionOut, ":", ":h=", 1)
		case transcoding.QSV:
			scaleFilter = "scale_qsv=w=" + strings.Replace(job.ResolutionOut, ":", ":h=", 1)
		default:
			scaleFilter = "scale=" + job.ResolutionOut
		}
		args = append(args, "-vf", scaleFilter)
	}

	outBase := filepath.Join(t.outputDir, job.ID)

	audioCodec := job.AudioCodec
	if audioCodec == "" {
		audioCodec = transcoding.DefaultAudioCodec
	}
	args = append(args,
		"-c:a", audioCodec,
		"-f", "hls",
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_segment_filename", outBase+"_%03d.ts",
		outBase+".m3u8",
	)

	return args
}

func mapVideoEncoder(codec, hwAccel string) string {
	base := strings.ToLower(codec)
	switch transcoding.HWAccelType(hwAccel) {
	case transcoding.VAAPI:
		switch base {
		case "h264", "libx264":
			return "h264_vaapi"
		case "hevc", "h265", "libx265":
			return "hevc_vaapi"
		}
	case transcoding.NVENC:
		switch base {
		case "h264", "libx264":
			return "h264_nvenc"
		case "hevc", "h265", "libx265":
			return "hevc_nvenc"
		}
	case transcoding.QSV:
		switch base {
		case "h264", "libx264":
			return "h264_qsv"
		case "hevc", "h265", "libx265":
			return "hevc_qsv"
		}
	}

	switch base {
	case "h264":
		return "libx264"
	case "hevc", "h265":
		return "libx265"
	case "vp9":
		return "libvpx-vp9"
	case "av1":
		return "libaom-av1"
	}
	return codec
}

func ParseProgress(line string, jobID string, durationMs *int64) (*transcoding.TranscodeProgress, bool) {
	m := reProgress.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}

	fps, _ := strconv.ParseFloat(m[2], 64)
	hours, _ := strconv.Atoi(m[3])
	mins, _ := strconv.Atoi(m[4])
	secs, _ := strconv.Atoi(m[5])
	centis, _ := strconv.Atoi(m[6])

	currentMs := int64(hours*3600000 + mins*60000 + secs*1000 + centis*10)
	bitrate, _ := strconv.ParseFloat(m[7], 64)
	speed, _ := strconv.ParseFloat(m[8], 64)

	p := &transcoding.TranscodeProgress{
		JobID:          jobID,
		FPS:            fps,
		Speed:          speed,
		CurrentBitrate: int64(bitrate * 1000),
	}

	if durationMs != nil && *durationMs > 0 {
		p.Percent = float64(currentMs) / float64(*durationMs) * 100
		if p.Percent > 100 {
			p.Percent = 100
		}
		remaining := float64(*durationMs-currentMs) / 1000
		if speed > 0 {
			p.ETAMs = int64(remaining / speed * 1000)
		}
	}

	return p, true
}

// scanFFmpegLines splits on \r or \n (ffmpeg uses \r for progress)
func scanFFmpegLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
