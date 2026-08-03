package offlineaudio

// join.go - reassemble a size-reduced download from the Matroska slices the
// server handed over.
//
// The server never publishes a whole 720p file: it emits consecutive slices
// out of ONE ffmpeg encoder and releases each as the phone collects it, so
// there is no second copy of the film anywhere. Putting the film back together
// is therefore the device's job, and this is it.
//
// It is mkvgo's ops.Join: it refuses slices whose CodecPrivate differs (which
// would silently produce an unplayable file), keeps the tracks aligned across
// the seams, and - since v0.26.0 - declares the length it computed instead of
// the first slice's (the v0.25.1 defect this package used to repair with an
// EditInPlace stamp after the join).
//
// Every exported signature sticks to gomobile's type subset, like session.go.
// durationMs stays in JoinSlices' signature for that reason: dropping it would
// churn the JNI/Kotlin/TS chain for nothing (the Int/long drift already broke
// one APK build).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/ops"
)

// Stable error prefixes, same contract as session.go: gomobile carries the Go
// error across JNI as an exception message, so the message IS the API.
const (
	errNoSlices     = "no-slices"
	errSliceMissing = "slice-missing"
	errJoinFailed   = "join-failed"
	// The pass's own cancellation is raised on the Kotlin side; the join's is
	// raised here (only Go knows where the slice boundaries are), so it needs
	// the code spelled out to land on the same JS branch.
	errCancelled = "cancelled"
)

// SliceName is the on-disk name of slice index, matching what the server's
// segment muxer produced and what the downloader saved. Kept here so the Go
// and TypeScript sides cannot drift apart on the naming.
func SliceName(index int) string { return fmt.Sprintf("slice-%05d.mkv", index) }

// joinCancelled is set by CancelJoin and read between slices. One join at a
// time is a caller invariant (the JS queue enforces it), so a package-level
// flag is enough - same posture as the pass's own cancel.
var joinCancelled atomic.Bool

// CancelJoin asks the running join to stop at the next slice boundary. The
// half-written output is removed by JoinSlices before it returns.
func CancelJoin() { joinCancelled.Store(true) }

// joinProgress is the running join's completion, 0..1000. Polled from Kotlin
// rather than pushed through a gomobile callback, exactly like the pass's
// remux progress - and for the same hard-won reason: a multi-gigabyte copy
// behind a label that never moves reads as a hang, and it has.
var joinProgress atomic.Int64

// JoinPermille is how far the running join has got, 0..1000.
func JoinPermille() int64 { return joinProgress.Load() }

// JoinSlices assembles dir/slice-00000.mkv .. slice-<count-1>.mkv into
// outPath. durationMs is unused since mkvgo v0.26.0 writes the computed total
// itself; it stays for JNI signature stability (see the file header).
//
// It does NOT delete the slices: the caller does that once the join has
// succeeded. Keeping them until then is what makes an interrupted assembly
// recoverable - the alternative loses an hour of server encoding and a
// multi-gigabyte transfer to one process kill.
//
// outPath is left absent on any failure, so its existence means "complete".
func JoinSlices(dir string, count int, outPath string, durationMs int64) (err error) {
	joinCancelled.Store(false)
	if count <= 0 {
		return fmt.Errorf("%s: nothing to join", errNoSlices)
	}
	sources := make([]string, 0, count)
	for i := range count {
		p := filepath.Join(dir, SliceName(i))
		if _, statErr := os.Stat(p); statErr != nil {
			return fmt.Errorf("%s: slice %d of %d is not on disk", errSliceMissing, i, count)
		}
		sources = append(sources, p)
	}

	// Write beside the target and rename: a killed join must never leave
	// something that looks like a finished download.
	tmp := outPath + ".joining"
	_ = os.Remove(tmp)
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	ctx := context.Background()
	joinProgress.Store(0)
	progress := mkv.Options{Progress: func(done, total int64) {
		if total > 0 {
			joinProgress.Store(min(done*1000/total, 999))
		}
	}}
	if err = ops.Join(ctx, sources, tmp, progress); err != nil {
		return fmt.Errorf("%s: %w", errJoinFailed, err)
	}
	if joinCancelled.Load() {
		return fmt.Errorf("%s: cancelled", errCancelled)
	}
	if err = os.Rename(tmp, outPath); err != nil {
		return fmt.Errorf("%s: %w", errJoinFailed, err)
	}
	joinProgress.Store(1000)
	return nil
}

// DeleteSlices removes the collected slices once the join has succeeded. Split
// from JoinSlices on purpose: the slices are the only thing standing between a
// crash and re-downloading the whole film, so they outlive the assembly and go
// only when there is a finished file to replace them.
func DeleteSlices(dir string, count int) {
	for i := range count {
		_ = os.Remove(filepath.Join(dir, SliceName(i)))
	}
	// Empty by now unless something else wrote in there; RemoveAll would be
	// the wrong tool if the caller ever shares the directory.
	_ = os.Remove(dir)
}
