package procfork

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestHasInputThreads(t *testing.T) {
	t.Parallel()
	if !hasInputThreads([]string{"-threads", "1", "-i", "f"}) {
		t.Fatal("decoder-side -threads must be detected")
	}
	if hasInputThreads([]string{"-i", "f", "-c:v", "libx264", "-threads", "2", "out"}) {
		t.Fatal("encoder-side -threads is not the decoder's")
	}
	if hasInputThreads(nil) {
		t.Fatal("empty")
	}
}

func TestIsMemoryFailure(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"[dec:hevc @ 0x1] Decoding error: Cannot allocate memory",
		"ffmpeg: error while loading shared libraries: libX11.so.6: failed to map segment from shared object",
		"Out of memory",
	} {
		if !isMemoryFailure([]byte(s)) {
			t.Fatalf("must match %q", s)
		}
	}
	if isMemoryFailure([]byte("Invalid data found when processing input")) {
		t.Fatal("a decode error is not a memory failure")
	}
}

func TestCommand_ArgvShape(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cmd, finish, err := Command(ctx, Options{Class: Scan, Site: "t"}, "ffmpeg", "-i", "f.mkv", "-f", "null", "-")
	if err != nil {
		t.Fatal(err)
	}
	defer finish(nil, nil)
	argv := cmd.Args
	i := slices.Index(argv, "ffmpeg")
	if i < 0 {
		t.Fatalf("binary missing from %v", argv)
	}
	if got := argv[i+1:]; !slices.Equal(got, []string{"-threads", "1", "-i", "f.mkv", "-f", "null", "-"}) {
		t.Fatalf("scan must decode on one thread, got %v", got)
	}
	pre := strings.Join(argv[:i], " ")
	if prlimitPath() != "" && !strings.HasPrefix(pre, "prlimit --as=") {
		t.Fatalf("the address-space cap must lead the prefix: %q", pre)
	}
	if nicePath() != "" && !strings.Contains(pre, "nice -n 19") {
		t.Fatalf("scan must be niced: %q", pre)
	}
	if ionicePath() != "" && !strings.Contains(pre, "ionice -c 3") {
		t.Fatalf("scan must be idle on disk: %q", pre)
	}
}

func TestCommand_CallerThreadsWin(t *testing.T) {
	t.Parallel()
	cmd, finish, err := Command(context.Background(), Options{Class: Scan, Site: "t"}, "ffmpeg", "-threads", "2", "-i", "f")
	if err != nil {
		t.Fatal(err)
	}
	defer finish(nil, nil)
	if n := strings.Count(strings.Join(cmd.Args, " "), "-threads"); n != 1 {
		t.Fatalf("-threads must appear once, argv %v", cmd.Args)
	}
}

func TestCommand_ProbeAndNoNiceAndNoMemCap(t *testing.T) {
	t.Parallel()
	cmd, finish, err := Command(context.Background(), Options{Class: Probe, Site: "t"}, "ffprobe", "-v", "quiet", "f")
	if err != nil {
		t.Fatal(err)
	}
	finish(nil, nil)
	if !slices.Equal(cmd.Args, []string{"ffprobe", "-v", "quiet", "f"}) {
		t.Fatalf("a probe runs untouched, got %v", cmd.Args)
	}
	cmd, finish, err = Command(context.Background(), Options{Class: Production, Site: "t", NoNice: true, NoMemCap: true, UnboundedThreads: true}, "ffmpeg", "-i", "f")
	if err != nil {
		t.Fatal(err)
	}
	finish(nil, nil)
	if !slices.Equal(cmd.Args, []string{"ffmpeg", "-i", "f"}) {
		t.Fatalf("NoNice+NoMemCap+UnboundedThreads runs untouched, got %v", cmd.Args)
	}
}

func TestFinish_WrapsMemoryCapFailure(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	cmd, finish, err := Command(context.Background(), Options{Class: Interactive, Site: "t", UnboundedThreads: true},
		"sh", "-c", "echo 'Decoding error: Cannot allocate memory' >&2; exit 245")
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := cmd.Output()
	got := finish(runErr, nil)
	if !errors.Is(got, ErrMemoryCapExceeded) {
		t.Fatalf("exit 245 + allocation error must be attributed to the cap, got %v", got)
	}
	var exitErr *exec.ExitError
	if !errors.As(got, &exitErr) {
		t.Fatal("the original exit error must stay reachable")
	}
}

func TestFinish_PlainFailureIsNotAMemoryFailure(t *testing.T) {
	t.Parallel()
	cmd, finish, err := Command(context.Background(), Options{Class: Interactive, Site: "t", UnboundedThreads: true},
		"sh", "-c", "echo 'Invalid data' >&2; exit 1")
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := cmd.Output()
	if got := finish(runErr, nil); errors.Is(got, ErrMemoryCapExceeded) {
		t.Fatal("must not be attributed to the cap")
	}
}

// A probe's "no" is its answer, not an incident: the exit lands at DEBUG and
// the caller reports the verdict. Interactive failures keep the WARN.
func TestFinish_ProbeFailureIsNotAWarning(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	for _, tc := range []struct {
		class Class
		want  string
	}{{Probe, "level=DEBUG"}, {Interactive, "level=WARN"}} {
		buf.Reset()
		cmd, finish, err := Command(context.Background(), Options{Class: tc.class, Site: "t", UnboundedThreads: true},
			"sh", "-c", "exit 1")
		if err != nil {
			t.Fatal(err)
		}
		_, runErr := cmd.Output()
		_ = finish(runErr, nil)
		line := buf.String()
		if !strings.Contains(line, "procfork: failed") || !strings.Contains(line, tc.want) {
			t.Errorf("class %v: want a %s failure line, got %q", tc.class, tc.want, line)
		}
	}
}
