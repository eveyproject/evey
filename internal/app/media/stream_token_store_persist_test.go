package media_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/app/media"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTokenRepo is the database a restarted server finds again.
type fakeTokenRepo struct {
	mu      sync.Mutex
	rows    map[string]persistedRow
	loads   int
	extends int
}

type persistedRow struct {
	st     shared.StreamToken
	issued time.Time
}

func newFakeTokenRepo() *fakeTokenRepo { return &fakeTokenRepo{rows: map[string]persistedRow{}} }

func (f *fakeTokenRepo) SaveStreamToken(_ context.Context, hash string, st shared.StreamToken, issued time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	st.Token = ""
	f.rows[hash] = persistedRow{st: st, issued: issued}
	return nil
}

func (f *fakeTokenRepo) LoadStreamToken(_ context.Context, hash string) (shared.StreamToken, time.Time, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	r, ok := f.rows[hash]
	return r.st, r.issued, ok, nil
}

func (f *fakeTokenRepo) ExtendStreamToken(_ context.Context, hash string, exp time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extends++
	if r, ok := f.rows[hash]; ok && exp.After(r.st.ExpiresAt) {
		r.st.ExpiresAt = exp
		f.rows[hash] = r
	}
	return nil
}

func (f *fakeTokenRepo) del(keep func(persistedRow) bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for h, r := range f.rows {
		if !keep(r) {
			delete(f.rows, h)
		}
	}
}

func (f *fakeTokenRepo) DeleteStreamToken(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, hash)
	return nil
}

func (f *fakeTokenRepo) DeleteStreamTokensByUser(_ context.Context, u shared.UserID) error {
	f.del(func(r persistedRow) bool { return r.st.UserID != u })
	return nil
}

func (f *fakeTokenRepo) DeleteStreamTokensByRoom(_ context.Context, room string) error {
	f.del(func(r persistedRow) bool { return r.st.RoomID != room })
	return nil
}

func (f *fakeTokenRepo) DeleteExpiredStreamTokens(_ context.Context, now time.Time) error {
	f.del(func(r persistedRow) bool { return !now.After(r.st.ExpiresAt) })
	return nil
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func stores(t *testing.T, repo *fakeTokenRepo, c *clock) (*media.StreamTokenStore, func() *media.StreamTokenStore) {
	t.Helper()
	first := media.NewStreamTokenStoreForTest(5*time.Minute, c.now).WithPersistence(repo)
	t.Cleanup(first.Stop)
	restart := func() *media.StreamTokenStore {
		s := media.NewStreamTokenStoreForTest(5*time.Minute, c.now).WithPersistence(repo)
		t.Cleanup(s.Stop)
		return s
	}
	return first, restart
}

// The production cut of 2026-09-23/24: the server restarted and every native
// player's token was refused. A restarted store must still know it.
func TestStreamTokenStore_TokenSurvivesARestart(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	before, restart := stores(t, repo, c)
	tok := before.CreateWithTTL("usr-1", "prf-1", "m1", 12*time.Hour)

	after := restart()
	st, ok := after.Validate(tok, "m1")
	require.True(t, ok)
	assert.Equal(t, shared.UserID("usr-1"), st.UserID)
	assert.Equal(t, shared.ProfileID("prf-1"), st.ProfileID)
	assert.Equal(t, 12*time.Hour, st.TTL, "the long native window is kept, not the store default")
	_, wrongMedia := after.Validate(tok, "other")
	assert.False(t, wrongMedia)
}

func TestStreamTokenStore_StoresAHashNeverTheToken(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	s, _ := stores(t, repo, c)
	tok := s.Create("usr-1", "", "m1")
	for hash, row := range repo.rows {
		assert.NotContains(t, hash, tok)
		assert.Empty(t, row.st.Token)
	}
	require.Len(t, repo.rows, 1)
}

// A revocation that only touched memory would come back at the next restart.
func TestStreamTokenStore_RevocationsOutliveARestart(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	before, restart := stores(t, repo, c)
	byUser := before.CreateWithTTL("usr-1", "", "m1", time.Hour)
	single := before.CreateWithTTL("usr-2", "", "m1", time.Hour)
	guest := before.CreateGuestToken("m1", "room-1", time.Hour)
	kept := before.CreateWithTTL("usr-3", "", "m1", time.Hour)

	before.RevokeByUser("usr-1")
	before.Revoke(single)
	before.RevokeByRoom("room-1")

	after := restart()
	for _, tok := range []string{byUser, single, guest} {
		_, ok := after.Validate(tok, "")
		assert.False(t, ok, "a revoked token must stay revoked")
	}
	_, ok := after.Validate(kept, "")
	assert.True(t, ok)
}

// Revoking a user reaches tokens the restarted server has not loaded yet.
func TestStreamTokenStore_RevokeByUserReachesUnloadedTokens(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	before, restart := stores(t, repo, c)
	tok := before.CreateWithTTL("usr-1", "", "m1", time.Hour)

	after := restart()
	after.RevokeByUser("usr-1")
	_, ok := restart().Validate(tok, "")
	assert.False(t, ok)
}

func TestStreamTokenStore_ExpiredTokenIsNotRevived(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	before, restart := stores(t, repo, c)
	tok := before.Create("usr-1", "", "m1")
	c.add(6 * time.Minute)
	_, ok := restart().Validate(tok, "")
	assert.False(t, ok)
}

func TestStreamTokenStore_AbsoluteCapHoldsAfterARestart(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	before, restart := stores(t, repo, c)
	tok := before.CreateWithTTL("usr-1", "", "m1", 12*time.Hour)
	// Kept slid for a day and a bit by steady traffic.
	for range 25 {
		c.add(time.Hour)
		before.Validate(tok, "")
	}
	_, ok := restart().Validate(tok, "")
	assert.False(t, ok, "24 h after minting, no restart brings it back")
}

// Made-up tokens must not become one query each.
func TestStreamTokenStore_UnknownTokenIsLookedUpOnce(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	s, _ := stores(t, repo, c)
	for range 5 {
		_, ok := s.Validate("not-a-token", "")
		assert.False(t, ok)
	}
	assert.Equal(t, 1, repo.loads)
	c.add(2 * time.Minute)
	s.Validate("not-a-token", "")
	assert.Equal(t, 2, repo.loads)
}

// Validate runs on every segment: the slid expiry is written back at most
// once a minute, and a restart then honours it.
func TestStreamTokenStore_SlidingExpiryIsPersistedSparingly(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	repo := newFakeTokenRepo()
	before, restart := stores(t, repo, c)
	tok := before.Create("usr-1", "", "m1")
	for range 20 {
		c.add(2 * time.Second)
		before.Validate(tok, "")
	}
	time.Sleep(50 * time.Millisecond)
	repo.mu.Lock()
	assert.LessOrEqual(t, repo.extends, 1)
	repo.mu.Unlock()

	// Played for 10 minutes on a 5-minute window, then the server restarts.
	for range 10 {
		c.add(time.Minute)
		before.Validate(tok, "")
	}
	require.Eventually(t, func() bool {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		for _, r := range repo.rows {
			return r.st.ExpiresAt.After(c.now().Add(3 * time.Minute))
		}
		return false
	}, time.Second, 10*time.Millisecond)
	_, ok := restart().Validate(tok, "")
	assert.True(t, ok)
}
