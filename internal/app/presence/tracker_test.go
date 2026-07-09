package presence

import (
	"testing"
	"time"
)

func newClockTracker(now *time.Time) *Tracker {
	t := NewTracker()
	t.nowFn = func() time.Time { return *now }
	return t
}

func TestTracker_TouchAndOnline(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	tr := newClockTracker(&now)

	tr.Touch(Entry{UserID: "usr-a", Username: "alice"})
	tr.Touch(Entry{UserID: "usr-b", Username: "bob"})

	online := tr.Online()
	if len(online) != 2 {
		t.Fatalf("want 2 online, got %d", len(online))
	}
	if tr.Count() != 2 {
		t.Fatalf("want count 2, got %d", tr.Count())
	}
}

func TestTracker_BlankUserIgnored(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	tr := newClockTracker(&now)
	tr.Touch(Entry{UserID: "", Username: "ghost"})
	if tr.Count() != 0 {
		t.Fatalf("blank user must not be tracked, got %d", tr.Count())
	}
}

func TestTracker_LatestSnapshotWins(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	tr := newClockTracker(&now)
	tr.Touch(Entry{UserID: "usr-a", Username: "alice", ProfileID: "prf-1", ClientIP: "10.0.0.1"})
	now = now.Add(time.Second)
	tr.Touch(Entry{UserID: "usr-a", Username: "alice", ProfileID: "prf-2", ClientIP: "10.0.0.2"})

	online := tr.Online()
	if len(online) != 1 {
		t.Fatalf("same user must collapse to one entry, got %d", len(online))
	}
	if online[0].ProfileID != "prf-2" || online[0].ClientIP != "10.0.0.2" {
		t.Fatalf("latest snapshot must win, got %+v", online[0])
	}
}

func TestTracker_StaleEntriesDropOff(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	tr := newClockTracker(&now)
	tr.Touch(Entry{UserID: "usr-a", Username: "alice"})

	// Just inside the window: still online.
	now = now.Add(DefaultWindow - time.Second)
	if tr.Count() != 1 {
		t.Fatalf("within window must stay online, got %d", tr.Count())
	}

	// Past the window: dropped, and GC'd from the map.
	now = now.Add(2 * time.Second)
	if got := tr.Count(); got != 0 {
		t.Fatalf("past window must drop off, got %d", got)
	}
	if len(tr.byUser) != 0 {
		t.Fatalf("stale entry must be GC'd from the map, still have %d", len(tr.byUser))
	}
}

func TestTracker_OnlineSortedByLastSeen(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	tr := newClockTracker(&now)
	tr.Touch(Entry{UserID: "usr-a", Username: "alice"})
	now = now.Add(time.Second)
	tr.Touch(Entry{UserID: "usr-b", Username: "bob"})

	online := tr.Online()
	if online[0].UserID != "usr-b" {
		t.Fatalf("most-recently-seen must sort first, got %s", online[0].UserID)
	}
}
