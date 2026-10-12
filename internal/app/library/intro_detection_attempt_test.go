package library

import (
	"context"
	"errors"
	"fmt"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/procfork"
)

// A decode that hit its memory cap reproduces on the same file, like a kernel
// kill; memory pressure on the host is the sweep's problem, not the episode's.
func TestFailureConsumesAttempt_MemoryErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"killed", domainlibrary.ErrDetectorKilled, true},
		{"budget spent", context.DeadlineExceeded, true},
		{"memory cap exceeded", procfork.ErrMemoryCapExceeded, true},
		{"memory pressure", procfork.ErrMemoryPressure, false},
		{"canceled", context.Canceled, false},
		{"io error", errors.New("read: input/output error"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := failureConsumesAttempt(tc.err); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A killed decode counts as an attempt only while the sweep itself is alive:
// an admin's Stop kills the same ffmpeg the same way, and the episode must not
// pay for it.
func TestAttemptConsumed_NotWhileTheSweepIsStopped(t *testing.T) {
	killed := fmt.Errorf("%w: /x.mkv", domainlibrary.ErrDetectorKilled)
	if !attemptConsumed(context.Background(), killed) {
		t.Fatal("a killed decode on a live sweep must consume an attempt")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if attemptConsumed(ctx, killed) {
		t.Fatal("a killed decode on a stopped sweep must not consume an attempt")
	}
}
