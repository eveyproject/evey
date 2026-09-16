package metadata

import (
	"sync"
	"testing"
)

// The admin UI polls a running job's errors while the download goroutine
// keeps appending: the read must be a snapshot the caller owns, never a view
// on the live slice.
func TestDownloadJob_ErrorsAreSnapshotAndRaceFree(t *testing.T) {
	j := &DownloadJob{ID: "job-1"}
	if got := j.ListErrors(); got == nil || len(got) != 0 {
		t.Fatalf("a fresh job lists an empty, non-nil slice, got %#v", got)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 25 {
				j.addError("boom")
				_ = j.ListErrors()
			}
		})
	}
	wg.Wait()
	if n := len(j.ListErrors()); n != 200 {
		t.Fatalf("errors = %d, want every append kept", n)
	}

	snap := j.ListErrors()
	snap[0] = "tampered"
	if j.ListErrors()[0] != "boom" {
		t.Fatal("ListErrors must hand out a copy: mutating it must not touch the job")
	}
	j.addError("late")
	if len(snap) != 200 {
		t.Fatal("a snapshot must not grow after the job appends")
	}
}
