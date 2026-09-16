package scanindex_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/pkg/scanindex"
)

func TestSameMtime_ExactMatch(t *testing.T) {
	t.Parallel()
	a := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if !scanindex.SameMtime(a, a) {
		t.Fatal("identical times must be equal")
	}
}

func TestSameMtime_TruncatesToMicroseconds(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 1, 1, 12, 0, 0, 123456789, time.UTC)
	truncated := base.Truncate(time.Microsecond)
	if !scanindex.SameMtime(base, truncated) {
		t.Fatal("ns-precision input must match µs-truncated stored value")
	}
}

func TestSameMtime_AbsorbsOneMicrosecondDrift(t *testing.T) {
	t.Parallel()
	a := time.Date(2025, 1, 1, 12, 0, 0, 847475000, time.UTC)
	b := a.Add(time.Microsecond)
	if !scanindex.SameMtime(a, b) {
		t.Fatal("1µs drift must not flag as changed (Postgres rounding)")
	}
	if !scanindex.SameMtime(b, a) {
		t.Fatal("1µs drift symmetric")
	}
}

func TestSameMtime_RejectsSecondScaleChange(t *testing.T) {
	t.Parallel()
	a := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	b := a.Add(time.Second)
	if scanindex.SameMtime(a, b) {
		t.Fatal("1s drift must flag as changed")
	}
}

func TestSameMtime_ZeroValueDoesNotOverflow(t *testing.T) {
	t.Parallel()
	var zero time.Time
	now := time.Now()
	// Regression: zero.Sub(now) saturates to MinInt64 which -x can't un-wrap.
	if scanindex.SameMtime(zero, now) {
		t.Fatal("zero vs now must not match")
	}
	if scanindex.SameMtime(now, zero) {
		t.Fatal("now vs zero must not match")
	}
}

func TestIndex_Unchanged_PathAndSizeAndMtimeMatch(t *testing.T) {
	t.Parallel()
	mod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/a.mp4", Size: 100, ModTime: mod},
	})
	id, ok := idx.Unchanged("/a.mp4", 100, mod)
	if !ok || id != "1" {
		t.Fatalf("want unchanged id=1, got ok=%v id=%s", ok, id)
	}
}

func TestIndex_Unchanged_SizeMismatch(t *testing.T) {
	t.Parallel()
	mod := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/a.mp4", Size: 100, ModTime: mod},
	})
	id, ok := idx.Unchanged("/a.mp4", 200, mod)
	if ok {
		t.Fatal("size mismatch must not be unchanged")
	}
	if id != "1" {
		t.Fatalf("existing ID still returned on size mismatch, got %s", id)
	}
}

func TestIndex_Unchanged_UnknownPath(t *testing.T) {
	t.Parallel()
	idx := scanindex.New(nil)
	id, ok := idx.Unchanged("/nope.mp4", 100, time.Now())
	if ok || id != "" {
		t.Fatalf("unknown path: want empty+false, got id=%s ok=%v", id, ok)
	}
}

func TestIndex_ExistingHash(t *testing.T) {
	t.Parallel()
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/a.mp4", Hash: "abc"},
		{ID: "2", Path: "/b.mp4", Hash: ""},
	})
	h, ok := idx.ExistingHash("/a.mp4")
	if !ok || h != "abc" {
		t.Fatalf("want abc,true; got %s,%v", h, ok)
	}
	h, ok = idx.ExistingHash("/b.mp4")
	if !ok || h != "" {
		t.Fatalf("empty hash on known path still reports found, got %s,%v", h, ok)
	}
	_, ok = idx.ExistingHash("/nope")
	if ok {
		t.Fatal("unknown path must report !found")
	}
}

func TestIndex_Moved_HashMatchAndPathFree(t *testing.T) {
	t.Parallel()
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/old.mp4", Hash: "abc"},
	})
	id, ok := idx.Moved("/new.mp4", "abc")
	if !ok || id != "1" {
		t.Fatalf("want id=1, got id=%s ok=%v", id, ok)
	}
}

func TestIndex_Moved_PathAlreadyOccupied(t *testing.T) {
	t.Parallel()
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/old.mp4", Hash: "abc"},
		{ID: "2", Path: "/new.mp4", Hash: "xyz"},
	})
	// A row already lives at /new.mp4 - incoming isn't a move, it's a
	// new-content-at-same-path situation; Moved must decline.
	id, ok := idx.Moved("/new.mp4", "abc")
	if ok || id != "" {
		t.Fatalf("path-occupied must not report move, got id=%s ok=%v", id, ok)
	}
}

func TestIndex_Moved_EmptyHashNeverMatches(t *testing.T) {
	t.Parallel()
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/old.mp4", Hash: ""},
	})
	id, ok := idx.Moved("/new.mp4", "")
	if ok || id != "" {
		t.Fatalf("empty hash must never match, got id=%s ok=%v", id, ok)
	}
}

func TestIndex_Moved_HashUnknown(t *testing.T) {
	t.Parallel()
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/old.mp4", Hash: "abc"},
	})
	id, ok := idx.Moved("/new.mp4", "zzz")
	if ok || id != "" {
		t.Fatalf("unknown hash must not match, got id=%s ok=%v", id, ok)
	}
}

// A row flagged for reprobe is reprocessed even though its path/size/mtime
// match; an unflagged or unknown row is not.
func TestIndex_NeedsReprobe(t *testing.T) {
	t.Parallel()
	idx := scanindex.New([]scanindex.Summary{
		{ID: "1", Path: "/a.mp4", NeedsReprobe: true},
		{ID: "2", Path: "/b.mp4"},
	})
	if !idx.NeedsReprobe("/a.mp4") {
		t.Fatal("flagged row must need a reprobe")
	}
	if idx.NeedsReprobe("/b.mp4") {
		t.Fatal("unflagged row must not")
	}
	if idx.NeedsReprobe("/nope") {
		t.Fatal("unknown path must not")
	}
}
