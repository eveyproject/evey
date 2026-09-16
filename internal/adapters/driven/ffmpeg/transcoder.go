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
	"syscall"

	"github.com/evey-project/evey/internal/pkg/procfork"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
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

	jobCtx, cancel := detachedJobContext(ctx)
	// GPU drivers map address space far beyond RSS: no RLIMIT_AS on the HW path.
	hw := job.HWAccelType != "" && transcoding.HWAccelType(job.HWAccelType) != transcoding.None
	cmd, finish, err := procfork.Command(jobCtx, procfork.Options{Class: procfork.Production, Site: "transcoder.job", NoMemCap: hw}, t.binaryPath, args...)
	if err != nil {
		cancel()
		return "", err
	}

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
				slog.ErrorContext(jobCtx, "transcoder: panic in stderr reader", slog.Any("panic", r), slog.String("jobID", job.ID))
			}
			if err := finish(cmd.Wait(), nil); err != nil {
				slog.WarnContext(jobCtx, "transcoder wait", slog.String("jobID", job.ID), slog.Any("err", err))
			}
			t.jobs.Delete(job.ID)
			// Natural exit must release the job context too - WithCancel leaks
			// its resources until cancel is called, and nothing else calls it
			// on this path (Stop() is the only other caller).
			cancel()
		}()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 64*1024), 64*1024)
		scanner.Split(scanFFmpegLines)
		for scanner.Scan() {
			if jobCtx.Err() != nil {
				return
			}
			line := scanner.Text()
			var durationMs *int64
			if job.Duration != nil {
				ms := job.Duration.Milliseconds()
				durationMs = &ms
			}
			if p, ok := ParseProgress(line, job.ID, durationMs); ok {
				rj.mu.Lock()
				rj.progress = p
				rj.mu.Unlock()
			}
		}
	}()

	return job.ID, nil
}

// detachedJobContext derives the ffmpeg job's context: cancellable (Stop, and
// the natural-exit cleanup) but NOT tied to the caller's request context - a
// batch transcode outlives the RPC that started it, and the progress stream
// closing must not kill an encode mid-file.
func detachedJobContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.WithoutCancel(ctx))
}

// StopAll cancels every in-flight job: the shutdown counterpart of
// detachedJobContext. Detached jobs survive their CALLER's context by design;
// they must not survive the SERVER - without this, a SIGTERM mid-encode
// leaves an orphaned ffmpeg running after the process exits.
func (t *Transcoder) StopAll() {
	t.jobs.Range(func(_, value any) bool {
		if rj, ok := value.(*runningJob); ok {
			rj.cancel()
		}
		return true
	})
}

func (t *Transcoder) Stop(_ context.Context, jobID string) error {
	v, ok := t.jobs.Load(jobID)
	if !ok {
		return fmt.Errorf("job %s: %w", jobID, shared.ErrNotFound)
	}
	rj, ok2 := v.(*runningJob)
	if !ok2 {
		return fmt.Errorf("job %s: invalid type", jobID)
	}
	rj.cancel()
	t.jobs.Delete(jobID)
	return nil
}

// Pause suspends the job's ffmpeg process (SIGSTOP) so it releases the CPU while
// keeping its progress; Resume (SIGCONT) continues it. ffmpeg is a single
// process here (threads, not child processes), so signalling its PID stops the
// whole encode.
func (t *Transcoder) Pause(_ context.Context, jobID string) error {
	return t.signal(jobID, syscall.SIGSTOP)
}

func (t *Transcoder) Resume(_ context.Context, jobID string) error {
	return t.signal(jobID, syscall.SIGCONT)
}

func (t *Transcoder) signal(jobID string, sig syscall.Signal) error {
	v, ok := t.jobs.Load(jobID)
	if !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	rj, ok2 := v.(*runningJob)
	if !ok2 {
		return fmt.Errorf("job %s: invalid type", jobID)
	}
	if rj.cmd == nil || rj.cmd.Process == nil {
		return fmt.Errorf("job %s: no live process", jobID)
	}
	return rj.cmd.Process.Signal(sig)
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
		case "av1", "libaom-av1", "libsvtav1":
			return "av1_vaapi"
		}
	case transcoding.NVENC:
		switch base {
		case "h264", "libx264":
			return "h264_nvenc"
		case "hevc", "h265", "libx265":
			return "hevc_nvenc"
		case "av1", "libaom-av1", "libsvtav1":
			return "av1_nvenc"
		}
	case transcoding.QSV:
		switch base {
		case "h264", "libx264":
			return "h264_qsv"
		case "hevc", "h265", "libx265":
			return "hevc_qsv"
		case "av1", "libaom-av1", "libsvtav1":
			return "av1_qsv"
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
		// SVT-AV1: libaom-av1 is absent from the shipped jellyfin-ffmpeg build
		// and far too slow. (The caller-supplied job.Preset must be numeric for
		// SVT - named x264 presets fail to open the encoder.)
		return "libsvtav1"
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
