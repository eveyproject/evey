package procfork

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/sysinfo"
)

// ErrMemoryCapExceeded wraps the exit of a process that hit its RLIMIT_AS:
// ffmpeg fails its allocation and exits on its own ("Cannot allocate memory"),
// a clean, attributable failure instead of a kernel OOM kill.
var ErrMemoryCapExceeded = errors.New("procfork: memory cap exceeded")

// Options selects the policy for one process.
type Options struct {
	Class Class
	// Site names the call site in the exit log ("trickplay.extract_frame").
	Site   string
	Source Source
	// NoNice keeps the default scheduling priority: a viewer is waiting on
	// the output (legacy playback paths).
	NoNice bool
	// NoMemCap skips RLIMIT_AS: GPU drivers map address space far beyond RSS.
	NoMemCap bool
	// UnboundedThreads leaves ffmpeg's thread auto-detection: calibration must
	// measure the whole host.
	UnboundedThreads bool
}

// Finish must be called once the process has been waited for. It releases the
// admission, logs the exit and returns runErr, wrapped in ErrMemoryCapExceeded
// when the cap is what stopped it. stderr may be nil when the caller let
// exec capture it (Output).
type Finish func(runErr error, stderr []byte) error

var (
	prlimitPath = sync.OnceValue(func() string { return lookPath("prlimit") })
	ionicePath  = sync.OnceValue(func() string { return lookPath("ionice") })
	nicePath    = sync.OnceValue(func() string { return lookPath("nice") })

	cpuBudget = sync.OnceValue(sysinfo.CPUBudget)
	memBudget = sync.OnceValue(func() int64 {
		b, ok := sysinfo.MemoryBudgetBytes()
		if !ok {
			return 0
		}
		return b
	})

	defaultAdmission = newAdmission(sysinfo.MemAvailableBytes, sysinfo.MemoryBudgetBytes)
)

func lookPath(bin string) string {
	p, err := exec.LookPath(bin)
	if err != nil {
		return ""
	}
	return p
}

// Command builds the exec.Cmd for bin+args under the class policy. It blocks
// while a Scan waits for memory and returns ErrMemoryPressure when admission
// is refused. The returned Finish must be called after Wait.
func Command(ctx context.Context, o Options, bin string, args ...string) (*exec.Cmd, Finish, error) {
	if o.Class == 0 {
		o.Class = Probe
	}
	rssCap := int64(0)
	if !o.NoMemCap {
		rssCap = RSSCap(o.Class, memBudget())
	}
	release, err := defaultAdmission.admit(ctx, o.Class, o.Site, RSSCap(o.Class, memBudget()))
	if err != nil {
		return nil, nil, err
	}

	threads := 0
	if o.Class != Probe && !o.UnboundedThreads && !hasInputThreads(args) {
		threads = Threads(o.Class, o.Source, cpuBudget(), rssCap)
		args = append([]string{"-threads", strconv.Itoa(threads)}, args...)
	}

	argv := prefix(o, rssCap)
	argv = append(argv, bin)
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // fixed binaries, args built by the callers

	started := time.Now()
	track.add(cmd, &flight{cmd: cmd, site: o.Site, class: o.Class, task: bgtasks.TaskFromContext(ctx), started: started})
	var once sync.Once
	var result error
	finish := func(runErr error, stderr []byte) error {
		once.Do(func() {
			release()
			track.remove(cmd, cpuSeconds(cmd))
			result = logExit(ctx, o, cmd, runErr, stderr, rssCap, threads, time.Since(started))
		})
		return result
	}
	return cmd, finish, nil
}

// cpuSeconds is the user+system CPU time the finished process consumed, from
// the wait rusage; 0 when the process never ran.
func cpuSeconds(cmd *exec.Cmd) float64 {
	ps := cmd.ProcessState
	if ps == nil {
		return 0
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok || ru == nil {
		return 0
	}
	return float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6
}

// prefix is [prlimit --as=N] [ionice -c 3] [nice -n 19]: the address-space
// cap first, so it applies to every exec below it.
func prefix(o Options, rssCap int64) []string {
	var argv []string
	if rssCap > 0 && prlimitPath() != "" {
		argv = append(argv, "prlimit", "--as="+strconv.FormatInt(addressSpaceCap(rssCap), 10))
	}
	if o.NoNice || o.Class == Probe {
		return argv
	}
	if o.Class == Scan && ionicePath() != "" {
		argv = append(argv, "ionice", "-c", "3")
	}
	if nicePath() != "" {
		argv = append(argv, "nice", "-n", "19")
	}
	return argv
}

// hasInputThreads reports a -threads set by the caller on the input side
// (before the first -i), which is the decoder's and must win.
func hasInputThreads(args []string) bool {
	for _, a := range args {
		if a == "-i" {
			return false
		}
		if a == "-threads" || strings.HasPrefix(a, "-threads:") {
			return true
		}
	}
	return false
}

func logExit(ctx context.Context, o Options, cmd *exec.Cmd, runErr error, stderr []byte, rssCap int64, threads int, wall time.Duration) error {
	attrs := []slog.Attr{
		slog.String("site", o.Site),
		slog.String("class", o.Class.String()),
		slog.String("argv", strings.Join(cmd.Args, " ")),
		slog.Int64("wall_ms", wall.Milliseconds()),
	}
	if threads > 0 {
		attrs = append(attrs, slog.Int("threads", threads))
	}
	if rssCap > 0 {
		attrs = append(attrs, slog.Int64("memcap_mb", rssCap>>20))
	}
	if ps := cmd.ProcessState; ps != nil {
		attrs = append(attrs, slog.Int("exit", ps.ExitCode()))
		if ru, ok := ps.SysUsage().(*syscall.Rusage); ok && ru != nil {
			attrs = append(attrs, slog.Int64("maxrss_mb", ru.Maxrss>>10), slog.Float64("cpu_s", cpuSeconds(cmd)))
		}
		if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			attrs = append(attrs, slog.String("signal", ws.Signal().String()))
		}
	}
	if stderr == nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			stderr = exitErr.Stderr
		}
	}
	if runErr == nil {
		level := slog.LevelDebug
		if o.Class == Interactive || o.Class == Production {
			level = slog.LevelInfo
		}
		slog.LogAttrs(ctx, level, "procfork: exited", attrs...)
		return nil
	}
	if rssCap > 0 && isMemoryFailure(stderr) {
		runErr = errors.Join(ErrMemoryCapExceeded, runErr)
	}
	attrs = append(attrs, slog.Any("err", runErr), slog.String("stderr_tail", tail(stderr, 300)))
	level := slog.LevelWarn
	// A probe exists to find out whether something works: "no" is an answer,
	// not an incident. The caller reports the verdict at its own level.
	if ctx.Err() != nil || o.Class == Probe {
		level = slog.LevelDebug
	}
	slog.LogAttrs(ctx, level, "procfork: failed", attrs...)
	return runErr
}

// isMemoryFailure recognises how a process reports its RLIMIT_AS: ffmpeg's
// allocation error, or the dynamic loader when the cap sits below the
// libraries themselves.
func isMemoryFailure(stderr []byte) bool {
	s := strings.ToLower(string(stderr))
	return strings.Contains(s, "cannot allocate memory") ||
		strings.Contains(s, "out of memory") ||
		strings.Contains(s, "failed to map segment")
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = s[len(s)-n:]
	}
	return s
}

// Output is Command + exec.Cmd.Output + Finish for the callers that only need
// stdout.
func Output(ctx context.Context, o Options, bin string, args ...string) ([]byte, error) {
	cmd, finish, err := Command(ctx, o, bin, args...)
	if err != nil {
		return nil, err
	}
	out, runErr := cmd.Output()
	return out, finish(runErr, nil)
}

// CombinedOutput is Command + exec.Cmd.CombinedOutput + Finish.
func CombinedOutput(ctx context.Context, o Options, bin string, args ...string) ([]byte, error) {
	cmd, finish, err := Command(ctx, o, bin, args...)
	if err != nil {
		return nil, err
	}
	out, runErr := cmd.CombinedOutput()
	return out, finish(runErr, out)
}

// Run is Command + exec.Cmd.Run + Finish, stderr captured for the log.
func Run(ctx context.Context, o Options, bin string, args ...string) error {
	_, err := CombinedOutput(ctx, o, bin, args...)
	return err
}
