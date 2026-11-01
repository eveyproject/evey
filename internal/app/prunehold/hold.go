// Package prunehold is the mass-disappearance guard shared by every scan that
// deletes rows whose file is gone (video, music, photo). A mount that is
// healthy but lost a branch (a union pool missing one disk) makes files vanish
// for real from the root's view, which the empty-root checks cannot tell from
// a deletion: past a share of a root, the deletion is held until an admin
// confirms that exact set of missing files.
package prunehold

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Guard bounds how much of a root may vanish in one pass before the prune
// stops deleting under it.
type Guard struct {
	// MaxMissingRatio is the share of a root's items (0.05 = 5 %) that may be
	// absent in one pass; above it, and past MinMissing, the root is held.
	MaxMissingRatio float64
	// MinMissing is the absolute floor: fewer missing files never suspend,
	// so a small library can still lose a few titles.
	MinMissing int
}

// DefaultGuard is 5 % and 20 files.
var DefaultGuard = Guard{MaxMissingRatio: 0.05, MinMissing: 20}

// DefaultConfirmTTL is how long a confirmation waits for the pass it allows.
// The confirm route starts that pass itself, so a confirmation still unused
// after this is one whose pass never reached the prune.
const DefaultConfirmTTL = 10 * time.Minute

// Suspension is a root whose deletions are held back. Snapshot identifies the
// exact set of missing items: a confirmation names it, and only that set may
// then be deleted.
type Suspension struct {
	LibraryID string
	Root      string
	Missing   int
	Total     int
	Snapshot  string
}

var (
	// ErrNothingSuspended: the root has no held deletion to confirm.
	ErrNothingSuspended = errors.New("no deletion is suspended under this root")
	// ErrSnapshotChanged: the held set is no longer the one the admin saw.
	ErrSnapshotChanged = errors.New("the missing files changed since this page was loaded")
)

type confirmation struct {
	snapshot string
	at       time.Time
}

// Hold keeps the suspended roots and the pending confirmations in memory. A
// restart forgets both: the next pass re-suspends and re-announces once, which
// is safe, and a lost confirmation only means the admin confirms again.
type Hold struct {
	events shared.EventBus
	guard  Guard
	ttl    time.Duration
	now    func() time.Time

	mu        sync.Mutex
	suspended map[string]map[string]Suspension   // library -> root -> state
	confirmed map[string]map[string]confirmation // library -> root -> pending
}

// New returns a hold with the default guard; events may be nil (no notice).
func New(events shared.EventBus) *Hold {
	return &Hold{
		events:    events,
		guard:     DefaultGuard,
		ttl:       DefaultConfirmTTL,
		now:       time.Now,
		suspended: make(map[string]map[string]Suspension),
		confirmed: make(map[string]map[string]confirmation),
	}
}

// WithGuard overrides the mass-disappearance thresholds.
func (h *Hold) WithGuard(g Guard) *Hold {
	h.guard = g
	return h
}

// WithClock overrides the clock and the confirmation lifetime (tests).
func (h *Hold) WithClock(now func() time.Time, ttl time.Duration) *Hold {
	h.now, h.ttl = now, ttl
	return h
}

// Snapshot fingerprints a set of missing item keys, order-free.
func Snapshot(keys []string) string {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:16])
}

// Suspensions lists the held roots of one library, by root.
func (h *Hold) Suspensions(libraryID string) []Suspension {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Suspension, 0, len(h.suspended[libraryID]))
	for _, s := range h.suspended[libraryID] {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out
}

// All lists the held roots of every library.
func (h *Hold) All() []Suspension {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Suspension
	for _, roots := range h.suspended {
		for _, s := range roots {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LibraryID != out[j].LibraryID {
			return out[i].LibraryID < out[j].LibraryID
		}
		return out[i].Root < out[j].Root
	})
	return out
}

// Confirm allows the next pass over one root to delete the held set the admin
// saw, named by its snapshot. It arms nothing when the root is not held or
// when the held set has changed since. The confirmation is used by the next
// pass that judges the root, whatever its verdict, or lapses after the TTL.
func (h *Hold) Confirm(libraryID, root, snapshot string) error {
	root = filepath.Clean(root)
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.suspended[libraryID][root]
	if !ok {
		return ErrNothingSuspended
	}
	if snapshot == "" || s.Snapshot != snapshot {
		return ErrSnapshotChanged
	}
	if h.confirmed[libraryID] == nil {
		h.confirmed[libraryID] = make(map[string]confirmation)
	}
	h.confirmed[libraryID][root] = confirmation{snapshot: snapshot, at: h.now()}
	return nil
}

// Abandon ends a pass that stopped before judging: the confirmations pending
// on the root (every root of the library when root is "") are dropped, so a
// later pass cannot inherit them.
func (h *Hold) Abandon(libraryID, root string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if root == "" {
		delete(h.confirmed, libraryID)
		return
	}
	delete(h.confirmed[libraryID], filepath.Clean(root))
}

// Judge decides one root of one pass: total is the number of items the
// library has under it, missing the keys of those this pass would delete. It
// reports whether they may go. Above the guard they are held unless a live
// confirmation names exactly this set; the root's confirmation is used up
// either way. A newly held root is logged and announced once.
func (h *Hold) Judge(ctx context.Context, libraryID, root string, total int, missing []string) bool {
	root = filepath.Clean(root)
	m := len(missing)

	h.mu.Lock()
	c, hadConfirm := h.confirmed[libraryID][root]
	delete(h.confirmed[libraryID], root)
	if len(h.confirmed[libraryID]) == 0 {
		delete(h.confirmed, libraryID)
	}
	if m < h.guard.MinMissing || float64(m) <= h.guard.MaxMissingRatio*float64(total) {
		h.clearLocked(libraryID, root)
		h.mu.Unlock()
		return true
	}
	snap := Snapshot(missing)
	if hadConfirm && c.snapshot == snap && h.now().Sub(c.at) <= h.ttl {
		h.clearLocked(libraryID, root)
		h.mu.Unlock()
		slog.WarnContext(ctx, "orphan prune: deleting the missing files an admin confirmed",
			slog.String("library_id", libraryID), slog.String("root", root), slog.Int("missing", m), slog.Int("total", total))
		return true
	}
	_, already := h.suspended[libraryID][root]
	s := Suspension{LibraryID: libraryID, Root: root, Missing: m, Total: total, Snapshot: snap}
	if h.suspended[libraryID] == nil {
		h.suspended[libraryID] = make(map[string]Suspension)
	}
	h.suspended[libraryID][root] = s
	h.mu.Unlock()

	if hadConfirm {
		slog.WarnContext(ctx, "orphan prune: confirmation ignored, the missing files are not the set the admin confirmed or it expired",
			slog.String("library_id", libraryID), slog.String("root", root), slog.Int("missing", m))
	}
	if !already {
		h.announce(ctx, s)
	}
	return false
}

func (h *Hold) clearLocked(libraryID, root string) {
	delete(h.suspended[libraryID], root)
	if len(h.suspended[libraryID]) == 0 {
		delete(h.suspended, libraryID)
	}
}

func (h *Hold) announce(ctx context.Context, s Suspension) {
	slog.WarnContext(ctx, "orphan prune suspended: too many files missing under a root (a mount missing a branch?); admin confirmation needed to delete",
		slog.String("library_id", s.LibraryID), slog.String("root", s.Root),
		slog.Int("missing", s.Missing), slog.Int("total", s.Total))
	if h.events == nil {
		return
	}
	evt := library.PruneSuspended{LibraryID: shared.LibraryID(s.LibraryID), Root: s.Root, Missing: s.Missing, Total: s.Total, OccurredAt: time.Now()}
	if err := h.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}
