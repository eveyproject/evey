package httpapi

import (
	"fmt"
	"os"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/procfork"
)

// TestMain fails the package when a memory admission is still booked once
// every test has returned and no child is running, as the ffmpeg package's
// does: the subtitle fallbacks here fork through procfork too, and a booking
// whose Finish was never called is subtracted from free memory for the life
// of the server.
func TestMain(m *testing.M) {
	code := m.Run()
	if booked := procfork.Reserved(); code == 0 && booked > 0 && len(procfork.Children()) == 0 {
		fmt.Fprintf(os.Stderr, "procfork: %d bytes still booked after the tests with no child in flight: a fork path returned without calling Finish\n", booked)
		code = 1
	}
	os.Exit(code)
}
