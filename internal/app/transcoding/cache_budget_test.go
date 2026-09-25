package transcoding

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCacheFile(t *testing.T, dir, name string, size int, mtime time.Time) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, make([]byte, size), 0o644))
	require.NoError(t, os.Chtimes(p, mtime, mtime))
	return p
}

// The files and the segment hashes share one budget and one clock: over the
// cap, the least recently used entry goes first whatever its kind, the
// in-flight .tmp is neither counted nor touched, and a compat MKV counts like
// a remux MP4 (the old janitor listed .mp4 only, so the compat cap never bit).
func TestCacheBudget_CapEvictsLeastRecentlyUsedAcrossKinds(t *testing.T) {
	c, _ := newTestCache(t)
	remux := t.TempDir()
	compat := t.TempDir()
	base := time.Now()
	oldFilm := writeCacheFile(t, compat, "compat-v1-aaaa.mkv", 300, base.Add(-3*time.Hour))
	writeSegments(t, c, "h-old", 3, 100) // 300
	backdateHash(t, c, "h-old", -2*time.Hour)
	midRemux := writeCacheFile(t, remux, "remux-v1-bbbb.mp4", 300, base.Add(-time.Hour))
	writeSegments(t, c, "h-new", 3, 100) // 300, written now: protected
	tmp := writeCacheFile(t, compat, "compat-v1-cccc.mkv.tmp", 1000, base.Add(-9*time.Hour))

	b := NewCacheBudget(c, NewRemuxCacheJanitor(remux), NewRemuxCacheJanitor(compat))
	// 1200 B over a 650 B cap, margin 50 (target 600): the two oldest entries
	// go, one of each kind.
	p := capOnly(650, 50)
	p.ProtectWindow = 10 * time.Minute
	b.sweepOnce(p.withDefaults(), quietLogger(), base)

	assert.NoFileExists(t, oldFilm, "oldest entry, a download: first out")
	assert.NoDirExists(t, filepath.Join(c.baseDir, "h-old"), "second oldest, a hash: out")
	assert.FileExists(t, midRemux, "under the target once the two oldest are gone")
	assert.DirExists(t, filepath.Join(c.baseDir, "h-new"))
	assert.FileExists(t, tmp, "a file being produced is never counted nor deleted")
}

func TestCacheBudget_UnderCap_NoOp(t *testing.T) {
	c, _ := newTestCache(t)
	dir := t.TempDir()
	a := writeCacheFile(t, dir, "remux-v1-aaaa.mp4", 100, time.Now().Add(-time.Hour))
	NewCacheBudget(c, NewRemuxCacheJanitor(dir)).sweepOnce(capOnly(1000, 0).withDefaults(), quietLogger(), time.Now())
	assert.FileExists(t, a)
}

// A missing directory (nothing produced yet) is not an error.
func TestCacheBudget_MissingDirIsFine(t *testing.T) {
	c, _ := newTestCache(t)
	require.NotPanics(t, func() {
		NewCacheBudget(c, NewRemuxCacheJanitor(filepath.Join(t.TempDir(), "absent"))).
			sweepOnce(capOnly(100, 0).withDefaults(), quietLogger(), time.Now())
	})
}

// A download nobody fetched for DownloadTTL goes whatever the cap; a segment
// hash never ages out this way (its life is the LRU under the cap), and a
// download fetched recently stays.
func TestCacheBudget_AgePassRetiresStaleDownloadsOnly(t *testing.T) {
	c, _ := newTestCache(t)
	compat := t.TempDir()
	base := time.Now()
	stale := writeCacheFile(t, compat, "compat-v1-aaaa.mkv", 100, base.Add(-8*24*time.Hour))
	fresh := writeCacheFile(t, compat, "compat-v1-bbbb.mkv", 100, base.Add(-6*24*time.Hour))
	writeSegments(t, c, "h", 2, 100)
	backdateHash(t, c, "h", -30*24*time.Hour)

	p := EvictionPolicy{MaxBytes: 1 << 40, MinFreeBytes: -1, DownloadTTL: 7 * 24 * time.Hour}
	NewCacheBudget(c, NewRemuxCacheJanitor(compat)).sweepOnce(p.withDefaults(), quietLogger(), base)

	assert.NoFileExists(t, stale)
	assert.FileExists(t, fresh)
	assert.DirExists(t, filepath.Join(c.baseDir, "h"), "the age pass is for downloads only")
}

// Under the free-space floor the sweeper evicts whatever the cap says, oldest
// first, until the floor plus the margin is back - only a hash a live session
// serves is spared. This is what keeps a small system disk from filling.
func TestCacheBudget_FloorPassIgnoresTheCapButNotLiveSessions(t *testing.T) {
	c, _ := newTestCache(t)
	compat := t.TempDir()
	base := time.Now()
	writeSegments(t, c, "live", 3, 100)
	backdateHash(t, c, "live", -3*time.Hour)
	release := c.RegisterSession("live")
	defer release()
	film := writeCacheFile(t, compat, "compat-v1-aaaa.mkv", 300, base.Add(-2*time.Hour))
	writeSegments(t, c, "idle", 3, 100)
	backdateHash(t, c, "idle", -time.Hour)
	writeSegments(t, c, "recent", 3, 100) // inside the protect window: the floor pass takes it anyway

	free := int64(100)
	p := EvictionPolicy{
		MaxBytes: 1 << 40, MarginBytes: 50, DownloadTTL: -1, ProtectWindow: 10 * time.Minute,
		MinFreeBytes: 500,
		FreeBytes:    func() (int64, error) { return free, nil },
	}
	NewCacheBudget(c, NewRemuxCacheJanitor(compat)).sweepOnce(p.withDefaults(), quietLogger(), base)

	assert.DirExists(t, filepath.Join(c.baseDir, "live"), "a live session is never cut")
	assert.NoFileExists(t, film, "oldest evictable: out (free 100 -> 400)")
	assert.NoDirExists(t, filepath.Join(c.baseDir, "idle"), "still under 550: out (free -> 700)")
	assert.DirExists(t, filepath.Join(c.baseDir, "recent"), "floor plus margin reached: the sweep stops")
}

func TestCacheBudget_FloorPassSkippedWhenFreeSpaceUnreadable(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "h", 2, 100)
	backdateHash(t, c, "h", -time.Hour)
	p := EvictionPolicy{MaxBytes: 1 << 40, DownloadTTL: -1, MinFreeBytes: 1 << 40,
		FreeBytes: func() (int64, error) { return 0, os.ErrPermission }}
	NewCacheBudget(c).sweepOnce(p.withDefaults(), quietLogger(), time.Now())
	assert.DirExists(t, filepath.Join(c.baseDir, "h"), "an unreadable statfs must not empty the cache")
}

// A file a player just pulled from is spared by the cap like a hash is: a
// remux is reopened on every range request, and evicting it under a cast
// session would make the next range pay a whole new remux.
func TestCacheBudget_CapSparesAFileServedWithinTheWindow(t *testing.T) {
	c, _ := newTestCache(t)
	remux := t.TempDir()
	base := time.Now()
	playing := writeCacheFile(t, remux, "remux-v2-aaaa.mp4", 1000, base.Add(-time.Minute))
	stale := writeCacheFile(t, remux, "remux-v2-bbbb.mp4", 300, base.Add(-time.Hour))

	b := NewCacheBudget(c, NewRemuxCacheJanitor(remux))
	p := capOnly(500, 50)
	p.ProtectWindow = 10 * time.Minute
	b.sweepOnce(p.withDefaults(), quietLogger(), base)

	assert.NoFileExists(t, stale, "past the window: evicted")
	assert.FileExists(t, playing, "served a minute ago: spared even over the cap")
}

// A remux a player holds survives every pass - past the download TTL, under
// the free-space floor, over the cap - while its unheld neighbour goes in
// each; released and past the linger, it is an ordinary file again.
func TestCacheBudget_LeasedFileSurvivesEveryPass(t *testing.T) {
	c, _ := newTestCache(t)
	remux := t.TempDir()
	base := time.Now()
	leases := NewFileLeases()
	b := NewCacheBudget(c, NewRemuxCacheJanitor(remux).WithLeases(leases))
	old := base.Add(-30 * 24 * time.Hour) // past the TTL, oldest of all

	passes := map[string]EvictionPolicy{
		"age":   {MaxBytes: 1 << 40, MinFreeBytes: -1, DownloadTTL: 24 * time.Hour},
		"floor": {MaxBytes: 1 << 40, DownloadTTL: -1, MinFreeBytes: 1 << 40, FreeBytes: func() (int64, error) { return 0, nil }},
		"cap":   capOnly(100, 0),
	}
	for name, p := range passes {
		playing := writeCacheFile(t, remux, "remux-v2-"+name+"-play.mp4", 1000, old)
		other := writeCacheFile(t, remux, "remux-v2-"+name+"-other.mp4", 1000, old)
		release := leases.Hold(playing)
		b.sweepOnce(p.withDefaults(), quietLogger(), base)
		assert.FileExists(t, playing, "%s pass: a held file stays", name)
		assert.NoFileExists(t, other, "%s pass: its unheld neighbour goes", name)
		release()
		b.sweepOnce(p.withDefaults(), quietLogger(), base.Add(leases.Linger+time.Second))
		assert.NoFileExists(t, playing, "%s pass: released and past the linger, it goes", name)
	}
}
