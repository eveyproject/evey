package procfork

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// Runs the real ffmpeg under a cap far below what a decode needs and checks
// the contract the whole design rests on: the process fails on its own, with
// a non-zero exit and no signal, and Finish attributes it to the cap.
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/pkg/procfork/ -run Integration -v
func TestIntegration_MemoryCapFailsCleanly(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (needs ffmpeg and prlimit)")
	}
	for _, bin := range []string{"ffmpeg", "prlimit"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
	ctx := context.Background()
	clip := filepath.Join(t.TempDir(), "clip.mkv")
	if err := Run(ctx, Options{Class: Probe, Site: "itest.gen"}, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=24:duration=2",
		"-pix_fmt", "yuv420p10le", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=none", clip); err != nil {
		t.Fatalf("generate clip: %v", err)
	}

	// Same prefix Command builds, with the cap forced under the decoder's needs
	// (600 MiB of address space: the libraries load, the frame pool does not).
	argv := []string{"prlimit", "--as=" + strconv.FormatInt(600*mib, 10), "ffmpeg", "-hide_banner", "-threads", "7", "-i", clip, "-an", "-f", "null", "-"}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // test argv
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Fatal("a decode under a 600 MiB address-space cap must fail")
	}
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || ws.Signaled() {
		t.Fatalf("the failure must be an exit, not a signal: %v", cmd.ProcessState)
	}
	if !isMemoryFailure(out) {
		t.Fatalf("stderr must carry the allocation error, got: %s", tail(out, 300))
	}

	// And through Command itself: the same failure comes back typed.
	cmdIn, finish, err := Command(ctx, Options{Class: Interactive, Site: "itest.cap", UnboundedThreads: true}, "sh", "-c",
		"exec "+cmd.String())
	if err != nil {
		t.Fatal(err)
	}
	out2, runErr2 := cmdIn.CombinedOutput()
	if got := finish(runErr2, out2); !errors.Is(got, ErrMemoryCapExceeded) {
		t.Fatalf("Finish must attribute the exit to the cap, got %v", got)
	}
}
