package ffmpeg

import (
	"context"
	"os/exec"
	"sync"
)

// lowPriorityPrefix demotes a background scan on BOTH axes: nice 19 only
// covers CPU, while trickplay generation (and any full-file scan) is an I/O
// profile - it reads the whole media file and writes sprite sheets. On a
// mechanical-disk NAS it is the read head, not the processor, that playback
// competes with, so the disk demotion is the one that matters there. ionice
// class 3 (idle) grants I/O only when nobody else is asking; a missing binary
// (or a scheduler that ignores it) degrades gracefully to nice-only.
var lowPriorityPrefix = sync.OnceValue(func() []string {
	if _, err := exec.LookPath("ionice"); err == nil {
		return []string{"ionice", "-c", "3", "nice", "-n", "19"}
	}
	return []string{"nice", "-n", "19"}
})

// lowPriorityCommand builds an exec.Cmd for bin+args wrapped in the CPU+disk
// low-priority prefix.
func lowPriorityCommand(ctx context.Context, bin string, args ...string) *exec.Cmd {
	prefix := lowPriorityPrefix()
	argv := make([]string, 0, len(prefix)+1+len(args))
	argv = append(argv, prefix[1:]...)
	argv = append(argv, bin)
	argv = append(argv, args...)
	return exec.CommandContext(ctx, prefix[0], argv...)
}
