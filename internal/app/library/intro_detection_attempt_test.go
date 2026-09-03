package library

import (
	"context"
	"errors"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/procfork"
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
