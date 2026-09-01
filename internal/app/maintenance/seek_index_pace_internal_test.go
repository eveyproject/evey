package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// pacedDiagnoser is the disk in these tests: every Diagnose costs a measurable
// amount of time (virtual, real, or both) and the fake records how many probes
// were in flight at once. cost advances the virtual clock, block is a real
// pause used only where concurrency has to be observable.
type pacedDiagnoser struct {
	mu       sync.Mutex
	inFlight int
	maxSeen  int
	calls    int

	clk   *virtualClock // optional: advanced by cost on every call
	cost  time.Duration
	block time.Duration
	// blockOnly restricts block to a single path, so a test can make exactly
	// one file expensive and observe what happens AFTER it.
	blockOnly string
	// truncateOdd marks one file in two as an incomplete download, so the
	// rollups differ from file to file instead of being uniform. Keyed on the
	// PATH, never on the call order - the whole point of the comparison is that
	// the order stops mattering.
	truncateOdd bool
}

// oddNumberedFile reports whether pNNN.mkv carries an odd number.
func oddNumberedFile(path string) bool {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	n, err := strconv.Atoi(strings.TrimPrefix(base, "p"))
	return err == nil && n%2 == 1
}

func (d *pacedDiagnoser) Diagnose(_ context.Context, path string) (*mkv.Diagnosis, error) {
	d.mu.Lock()
	d.calls++
	d.inFlight++
	if d.inFlight > d.maxSeen {
		d.maxSeen = d.inFlight
	}
	if d.clk != nil {
		d.clk.advance(d.cost)
	}
	d.mu.Unlock()

	if d.block > 0 && (d.blockOnly == "" || d.blockOnly == path) {
		time.Sleep(d.block)
	}

	d.mu.Lock()
	d.inFlight--
	d.mu.Unlock()

	out := &mkv.Diagnosis{Healthy: true}
	if d.truncateOdd && oddNumberedFile(path) {
		out.Healthy, out.Truncated, out.MissingTailBytes = false, true, 4096
	}
	return out, nil
}

func (d *pacedDiagnoser) peakConcurrency() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.maxSeen
}

func (d *pacedDiagnoser) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// virtualClock lets a pacing test assert exact durations without sleeping.
type virtualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newVirtualClock() *virtualClock {
	return &virtualClock{t: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *virtualClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *virtualClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// pauseLedger records what the scan asked to sleep instead of sleeping.
type pauseLedger struct {
	mu    sync.Mutex
	total time.Duration
	calls int
}

func (l *pauseLedger) sleep(context.Context, time.Duration) {}

func (l *pauseLedger) record(_ context.Context, d time.Duration) {
	l.mu.Lock()
	l.total += d
	l.calls++
	l.mu.Unlock()
}

func (l *pauseLedger) read() (time.Duration, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total, l.calls
}

// pacedFiles writes n real .mkv files: the scan stats every candidate and drops
// the ones it cannot read.
func pacedFiles(t *testing.T, n int) []MediaRef {
	t.Helper()
	dir := t.TempDir()
	out := make([]MediaRef, 0, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("p%03d.mkv", i))
		require.NoError(t, os.WriteFile(p, []byte("not-really-matroska"), 0o644))
		out = append(out, MediaRef{MediaID: fmt.Sprintf("m%03d", i), Title: fmt.Sprintf("t%03d", i), Path: p})
	}
	return out
}

func pacedService(files []MediaRef, diag *pacedDiagnoser) *SeekIndexService {
	return NewSeekIndexService(&stubReindexer{}, stubCatalog{files: files}, nil).WithDiagnoser(diag)
}

// The gentle pace exists so a film keeps playing while the library is
// diagnosed: for every second the scan spends reading a file it must stand
// aside for about a second, leaving the disks free half the time.
func TestScanGentlePace_StandsAsideAsLongAsItRead(t *testing.T) {
	files := pacedFiles(t, 5)
	clk := newVirtualClock()
	diag := &pacedDiagnoser{clk: clk, cost: 20 * time.Millisecond}
	ledger := &pauseLedger{}

	svc := pacedService(files, diag)
	svc.now = clk.now
	svc.sleep = ledger.record

	require.NoError(t, svc.StartScan(t.Context(), ""))
	waitJobIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	require.Equal(t, len(files), res.Total)

	paused, calls := ledger.read()
	probed := time.Duration(len(files)) * diag.cost
	assert.Equal(t, len(files), calls, "every freshly probed file must be followed by a pause")
	assert.InDelta(t, float64(probed), float64(paused), float64(5*time.Millisecond),
		"a gentle scan must idle about as long as it read (probed %s, paused %s)", probed, paused)
}

// The fast pace exists for a library spread over several disks: probing one
// file at a time leaves two of three spindles idle. It must run several probes
// at once - and never more than the bound, or it becomes the starvation it was
// meant to avoid.
func TestScanFastPace_ProbesSeveralFilesAtOnce(t *testing.T) {
	files := pacedFiles(t, 12)
	diag := &pacedDiagnoser{block: 40 * time.Millisecond}
	ledger := &pauseLedger{}

	svc := pacedService(files, diag)
	svc.sleep = ledger.record

	require.NoError(t, svc.StartScan(t.Context(), "", ScanRunOptions{Pace: ScanPaceFast}))
	waitJobIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	require.Equal(t, len(files), res.Total)

	peak := diag.peakConcurrency()
	assert.Greater(t, peak, 1, "the fast pace must overlap probes, it stayed sequential")
	assert.LessOrEqual(t, peak, scanFastWorkers, "the fast pace must stay bounded")

	_, calls := ledger.read()
	assert.Zero(t, calls, "the fast pace must not pause at all")
}

// A file whose diagnosis is reused read nothing from the media disk, so it owes
// the disk nothing back: pausing for it would double a scan that costs almost
// nothing (the usual case - between two scans virtually no file changed).
func TestScanGentlePace_ReusedFileEarnsNoPause(t *testing.T) {
	files := pacedFiles(t, 4)
	clk := newVirtualClock()
	diag := &pacedDiagnoser{clk: clk, cost: 20 * time.Millisecond}

	svc := pacedService(files, diag)
	svc.now = clk.now

	first := &pauseLedger{}
	svc.sleep = first.record
	require.NoError(t, svc.StartScan(t.Context(), ""))
	waitJobIdle(t, svc)
	_, firstCalls := first.read()
	require.Equal(t, len(files), firstCalls)

	second := &pauseLedger{}
	svc.sleep = second.record
	require.NoError(t, svc.StartScan(t.Context(), ""))
	waitJobIdle(t, svc)

	require.Equal(t, len(files), diag.callCount(), "the second scan must reuse every unchanged diagnosis")
	paused, calls := second.read()
	assert.Zero(t, calls, "a reused file must not trigger a pause (paused %s)", paused)
}

// Cancelling must land at once, pause or no pause: an operator who started a
// scan by mistake cannot be made to wait out an idle period that exists only to
// spare the disks.
func TestScanGentlePace_CancelInterruptsThePause(t *testing.T) {
	files := pacedFiles(t, 6)
	// ONE expensive file buys a one-second pause right after it, and every
	// other file is instant: what the clock measures after the cancel is the
	// pause and nothing else.
	diag := &pacedDiagnoser{block: time.Second, blockOnly: files[0].Path}
	svc := pacedService(files, diag)

	require.NoError(t, svc.StartScan(t.Context(), ""))
	require.Eventually(t, func() bool { return svc.Status().Done >= 1 }, 5*time.Second, 5*time.Millisecond,
		"the scan never finished its first file")

	require.True(t, svc.Cancel())
	start := time.Now()
	require.Eventually(t, func() bool { return !svc.Status().Running }, 5*time.Second, 5*time.Millisecond,
		"the cancelled scan never stopped")
	assert.Less(t, time.Since(start), 600*time.Millisecond,
		"cancellation waited out the pause instead of interrupting it")
}

// Cancelling the fast pace must end the job too. The workers leave as soon as
// the job is cancelled, so whoever hands them work has to watch the
// cancellation as well - a bare send into a channel nobody reads any more parks
// the scan for good and the job slot never frees.
func TestScanFastPace_CancelEndsTheJob(t *testing.T) {
	files := pacedFiles(t, 40)
	diag := &pacedDiagnoser{block: 20 * time.Millisecond}
	svc := pacedService(files, diag)

	require.NoError(t, svc.StartScan(t.Context(), "", ScanRunOptions{Pace: ScanPaceFast}))
	require.Eventually(t, func() bool { return svc.Status().Done >= 1 }, 5*time.Second, 5*time.Millisecond,
		"the scan never finished its first file")

	require.True(t, svc.Cancel())
	require.Eventually(t, func() bool { return !svc.Status().Running }, 5*time.Second, 5*time.Millisecond,
		"the cancelled fast scan never stopped")
	assert.Less(t, diag.callCount(), len(files), "a cancelled scan must stop early")
}

// The pace is a disk budget, never a difference in what the operator is told:
// the same library must produce the very same items, in the very same order,
// with the very same rollups whichever pace ran.
func TestScanPace_SameResultWhicheverPace(t *testing.T) {
	files := pacedFiles(t, 9)

	gentleDiag := &pacedDiagnoser{truncateOdd: true}
	gentle := pacedService(files, gentleDiag)
	gentle.sleep = (&pauseLedger{}).sleep
	require.NoError(t, gentle.StartScan(t.Context(), "", ScanRunOptions{Pace: ScanPaceGentle}))
	waitJobIdle(t, gentle)
	gentleRes, _, ok := gentle.CachedScan("")
	require.True(t, ok)

	fastDiag := &pacedDiagnoser{truncateOdd: true}
	fast := pacedService(files, fastDiag)
	require.NoError(t, fast.StartScan(t.Context(), "", ScanRunOptions{Pace: ScanPaceFast}))
	waitJobIdle(t, fast)
	fastRes, _, ok := fast.CachedScan("")
	require.True(t, ok)

	require.Equal(t, len(files), gentleRes.Total)
	assert.Equal(t, gentleRes, fastRes, "the two paces disagreed on the library's health")
}

// An unset pace is the gentle one: nobody watches a scan, everybody watches
// their films.
func TestScanRunOptions_DefaultsToGentle(t *testing.T) {
	assert.Equal(t, ScanPaceGentle, ScanRunOptions{}.pace())
	assert.Equal(t, ScanPaceGentle, ScanRunOptions{Pace: "nonsense"}.pace())
	assert.Equal(t, ScanPaceFast, ScanRunOptions{Pace: ScanPaceFast}.pace())
}
