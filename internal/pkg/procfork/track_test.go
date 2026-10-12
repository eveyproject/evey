package procfork

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/pkg/bgtasks"
)

// A fork started under a task-tagged context shows up in Children with that
// task while it runs, and is charged to the task's totals once it exits. This
// is the whole basis of the admin panel's per-task cost; the tools it uses
// are the same sleep(1) every CI image ships.
func TestTracker_AttributesForkToTask(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not in PATH")
	}
	// Totals are process-wide and cumulative: read the count before this fork,
	// so a rerun (-count>1) measures its own fork instead of the sum of all runs.
	before := Totals()["test_task_track"].Forks
	ctx := bgtasks.WithTask(context.Background(), "test_task_track")
	cmd, finish, err := Command(ctx, Options{Class: Probe, Site: "test.track"}, "sleep", "1")
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var seen *Child
	deadline := time.Now().Add(2 * time.Second)
	for seen == nil && time.Now().Before(deadline) {
		for _, c := range Children() {
			if c.Task == "test_task_track" && c.PID == cmd.Process.Pid {
				cc := c
				seen = &cc
			}
		}
		if seen == nil {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if seen == nil {
		t.Fatal("the running fork never appeared in Children")
	}
	if seen.Site != "test.track" || seen.Class != "probe" {
		t.Fatalf("attribution: %+v", seen)
	}
	if seen.RSSBytes <= 0 {
		t.Fatalf("a live process has resident memory, got %d", seen.RSSBytes)
	}

	runErr := cmd.Wait()
	if err := finish(runErr, nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range Children() {
		if c.PID == cmd.Process.Pid {
			t.Fatal("finished fork still listed")
		}
	}
	tot, ok := Totals()["test_task_track"]
	if !ok || tot.Forks != before+1 {
		t.Fatalf("totals must charge the task one fork, got %+v (%v)", tot, ok)
	}
}

func TestTracker_UntaggedForkIsUnattributed(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("true not in PATH")
	}
	before := Totals()[""].Forks
	if err := Run(context.Background(), Options{Class: Probe, Site: "test.untagged"}, "true"); err != nil {
		t.Fatal(err)
	}
	if got := Totals()[""].Forks; got != before+1 {
		t.Fatalf("the fork with no task must land under the empty key: %d -> %d", before, got)
	}
}
