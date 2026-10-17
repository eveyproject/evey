package ffmpeg

import (
	"fmt"
	"os"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/procfork"
)

// TestMain fails the package when a memory admission is still booked once
// every test has returned and no child is running. Every fork here goes
// through procfork and must hand its reservation back through Finish; a
// reservation that outlives its process is subtracted from the free memory
// for the life of the server, and three of them refuse every transcode with
// "not enough free memory" while the host sits idle (measured 2026-10-17: a
// route-bench session whose context had expired kept 4 GiB booked).
func TestMain(m *testing.M) {
	code := m.Run()
	if booked := procfork.Reserved(); code == 0 && booked > 0 && len(procfork.Children()) == 0 {
		fmt.Fprintf(os.Stderr, "procfork: %d bytes still booked after the tests with no child in flight: a fork path returned without calling Finish\n", booked)
		code = 1
	}
	os.Exit(code)
}
