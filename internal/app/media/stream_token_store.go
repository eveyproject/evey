package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/evey-project/evey/internal/pkg/bgloop"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/google/uuid"
)

const (
	DefaultStreamTokenTTL = 5 * time.Minute
	cleanupInterval       = 1 * time.Minute
	// CastStreamTokenTTL is the renewal window for tokens handed to a
	// Chromecast. The device fetches HLS segments on its own and buffers
	// minutes ahead, then goes quiet - with the 5-minute default it would
	// expire mid-film during a buffer-quiet stretch (the local player
	// dodges this by re-validating on every xhr, the cast can't). 12h
	// comfortably covers any single playback session.
	CastStreamTokenTTL = 12 * time.Hour
	// maxAbsoluteStreamTokenTTL is the hard ceiling on a token's lifetime,
	// independent of the sliding renewal in Validate. Without it a token kept
	// alive by regular traffic (e.g. a stuck cast receiver polling forever)
	// never dies and outlives any logout/ban that only touches refresh tokens.
	// 24h is comfortably above CastStreamTokenTTL (12h) so a legitimate single
	// cast/playback session is never cut mid-stream, while still bounding the
	// worst case to about a day.
	maxAbsoluteStreamTokenTTL = 24 * time.Hour

	// persistTimeout bounds every database call made for a token: a slow
	// database must not hold a segment request.
	persistTimeout = 2 * time.Second
	// persistSlideStep: the sliding expiry is written back at most this often
	// per token (Validate runs on every segment).
	persistSlideStep = time.Minute
	// missCacheTTL / missCacheMax: a token unknown to the database is not
	// looked up again for a while, so a burst of made-up tokens cannot turn
	// every media request into a query.
	missCacheTTL = time.Minute
	missCacheMax = 10000
	// lookupRate / lookupBurst cap the database lookups of unknown tokens for
	// the whole store: made-up tokens are free to send and each distinct one
	// escapes the miss cache. Above it a token is treated as unknown (not
	// cached), and a real player gets through on its next segment.
	lookupRate  = 50
	lookupBurst = 200
)

type StreamTokenStore struct {
	// tokens maps token string → *shared.StreamToken. CreateWithTTL is the ONLY
	// insert site (always a string key and a *shared.StreamToken value), so the
	// unchecked type assertions in Validate / cleanupLoop are invariant-safe by
	// construction - a bare v.(*shared.StreamToken) / key.(string) cannot panic.
	tokens sync.Map
	// issuedAt maps token string → time.Time, the instant the token was
	// minted. shared.StreamToken itself has no IssuedAt field (it's outside
	// this package's whitelist), so the absolute-lifetime cap is tracked here
	// instead, set once at creation and never mutated afterwards - same
	// race-free contract as tokens/now below.
	issuedAt sync.Map
	ttl      time.Duration
	done     chan struct{}
	// now is set once at construction and never mutated afterwards, so the
	// concurrent reads from the Validate hot path and the cleanupLoop
	// goroutine are race-free without a mutex.
	now func() time.Time

	// repo, when set (WithPersistence), keeps tokens across a restart. Set
	// once at wiring, before any request.
	repo         shared.StreamTokenRepository
	persistedExp sync.Map // token -> time.Time: the expiry last written to repo
	misses       sync.Map // token hash -> time.Time: unknown to repo until then
	missCount    atomic.Int64
	extendCh     chan tokenExtension
	lookups      *rate.Limiter
}

type tokenExtension struct {
	hash      string
	expiresAt time.Time
}

// hashToken is the key a token is stored under: never the token itself.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// WithPersistence makes tokens survive a server restart: each one is written
// to repo when minted, looked up there when an unknown token is presented, and
// removed there when revoked. Call once, before the store serves requests.
func (s *StreamTokenStore) WithPersistence(repo shared.StreamTokenRepository) *StreamTokenStore {
	s.repo = repo
	s.extendCh = make(chan tokenExtension, 256)
	s.lookups = rate.NewLimiter(lookupRate, lookupBurst)
	go s.extendLoop()
	return s
}

func (s *StreamTokenStore) persistCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), persistTimeout)
}

func (s *StreamTokenStore) save(st *shared.StreamToken, issued time.Time) {
	if s.repo == nil {
		return
	}
	ctx, cancel := s.persistCtx()
	defer cancel()
	if err := s.repo.SaveStreamToken(ctx, hashToken(st.Token), *st, issued); err != nil {
		// The token still works until the next restart.
		slog.Warn("stream token: could not be persisted", slog.Any("err", err))
		return
	}
	s.persistedExp.Store(st.Token, st.ExpiresAt)
}

// loadPersisted brings back a token minted before a restart.
func (s *StreamTokenStore) loadPersisted(token string) (*shared.StreamToken, bool) {
	if s.repo == nil || token == "" {
		return nil, false
	}
	hash := hashToken(token)
	now := s.now()
	if until, ok := s.misses.Load(hash); ok && now.Before(until.(time.Time)) {
		return nil, false
	}
	if !s.lookups.Allow() {
		return nil, false
	}
	ctx, cancel := s.persistCtx()
	defer cancel()
	st, issued, found, err := s.repo.LoadStreamToken(ctx, hash)
	if err != nil {
		slog.Warn("stream token: lookup failed", slog.Any("err", err))
		return nil, false
	}
	if !found || now.After(st.ExpiresAt) || now.After(issued.Add(maxAbsoluteStreamTokenTTL)) {
		if s.missCount.Add(1) > missCacheMax {
			s.misses.Clear()
			s.missCount.Store(0)
		}
		s.misses.Store(hash, now.Add(missCacheTTL))
		return nil, false
	}
	st.Token = token
	s.issuedAt.Store(token, issued)
	s.persistedExp.Store(token, st.ExpiresAt)
	s.tokens.Store(token, &st)
	return &st, true
}

func (s *StreamTokenStore) extendLoop() {
	for {
		select {
		case <-s.done:
			return
		case ext := <-s.extendCh:
			ctx, cancel := s.persistCtx()
			if err := s.repo.ExtendStreamToken(ctx, ext.hash, ext.expiresAt); err != nil {
				slog.Debug("stream token: expiry not persisted", slog.Any("err", err))
			}
			cancel()
		}
	}
}

// persistSlide writes a slid expiry back, at most once per persistSlideStep.
func (s *StreamTokenStore) persistSlide(token string, expiresAt time.Time) {
	if s.repo == nil {
		return
	}
	if last, ok := s.persistedExp.Load(token); ok && expiresAt.Sub(last.(time.Time)) < persistSlideStep {
		return
	}
	s.persistedExp.Store(token, expiresAt)
	select {
	case s.extendCh <- tokenExtension{hash: hashToken(token), expiresAt: expiresAt}:
	default: // Busy: the next slide writes it.
	}
}

// forgetDurably removes tokens from the repo: a revocation that only touched
// memory would come back to life at the next restart.
func (s *StreamTokenStore) forgetDurably(del func(ctx context.Context) error) {
	if s.repo == nil {
		return
	}
	ctx, cancel := s.persistCtx()
	defer cancel()
	if err := del(ctx); err != nil {
		slog.Error("stream token: revocation not persisted, the token survives a restart", slog.Any("err", err))
	}
}

func NewStreamTokenStore(ttl time.Duration) *StreamTokenStore {
	return NewStreamTokenStoreForTest(ttl, time.Now)
}

// NewStreamTokenStoreForTest builds a StreamTokenStore with an injected clock
// so expiry, sliding-TTL renewal and cleanup can be tested deterministically
// without time.Sleep. Production code should use NewStreamTokenStore.
func NewStreamTokenStoreForTest(ttl time.Duration, now func() time.Time) *StreamTokenStore {
	if ttl <= 0 {
		ttl = DefaultStreamTokenTTL
	}
	s := &StreamTokenStore{
		ttl:  ttl,
		done: make(chan struct{}),
		now:  now,
	}
	go s.cleanupLoop()
	return s
}

// Create, CreateWithTTL and CreateCast take shared.UserID / shared.ProfileID /
// shared.MediaItemID (not bare strings) so three consecutive same-typed
// arguments can no longer be silently swapped at a call site - a positional
// mix-up here mints a token bound to the wrong user/profile/media, which is a
// security bug (wrong parental scope, wrong owner), not just a wrong result.
func (s *StreamTokenStore) Create(userID shared.UserID, profileID shared.ProfileID, mediaItemID shared.MediaItemID) string {
	return s.CreateWithTTL(userID, profileID, mediaItemID, s.ttl)
}

// CreateWithTTL mints a token with an explicit sliding TTL. Used for cast
// playback, where the short default would expire during the Chromecast's
// buffer-quiet stretches. profileID may be empty (the streaming rate limiter
// then falls back to userID).
func (s *StreamTokenStore) CreateWithTTL(userID shared.UserID, profileID shared.ProfileID, mediaItemID shared.MediaItemID, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = s.ttl
	}
	token := uuid.New().String()
	s.issuedAt.Store(token, s.now())
	s.tokens.Store(token, &shared.StreamToken{
		Token:       token,
		UserID:      userID,
		ProfileID:   profileID,
		MediaItemID: mediaItemID,
		Scope:       shared.ScopeStream,
		ExpiresAt:   s.now().Add(ttl),
		TTL:         ttl,
	})
	s.persistNew(token)
	return token
}

// persistNew writes a freshly minted token to the repo.
func (s *StreamTokenStore) persistNew(token string) {
	if s.repo == nil {
		return
	}
	v, ok := s.tokens.Load(token)
	issued, ok2 := s.issuedAt.Load(token)
	if ok && ok2 {
		s.save(v.(*shared.StreamToken), issued.(time.Time))
	}
}

// CreateCast mints the long-TTL token handed to a cast receiver, marked so the
// cast progress tracker can key on its fetches (see shared.StreamToken.Cast).
func (s *StreamTokenStore) CreateCast(userID shared.UserID, profileID shared.ProfileID, mediaItemID shared.MediaItemID, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = s.ttl
	}
	token := uuid.New().String()
	s.issuedAt.Store(token, s.now())
	s.tokens.Store(token, &shared.StreamToken{
		Token:       token,
		UserID:      userID,
		ProfileID:   profileID,
		MediaItemID: mediaItemID,
		Scope:       shared.ScopeStream,
		Cast:        true,
		ExpiresAt:   s.now().Add(ttl),
		TTL:         ttl,
	})
	s.persistNew(token)
	return token
}

// CreateGuestToken mints a media-scoped token for an unauthenticated syncplay
// "watch together" guest (no user, no profile). It carries ScopeSyncplayGuest so
// the download handler (and any non-streaming endpoint) can refuse it while
// streaming the room's single media item still works.
func (s *StreamTokenStore) CreateGuestToken(mediaItemID, roomID string, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = s.ttl
	}
	token := uuid.New().String()
	s.issuedAt.Store(token, s.now())
	s.tokens.Store(token, &shared.StreamToken{
		Token:       token,
		MediaItemID: shared.MediaItemID(mediaItemID),
		RoomID:      roomID,
		Scope:       shared.ScopeSyncplayGuest,
		ExpiresAt:   s.now().Add(ttl),
		TTL:         ttl,
	})
	s.persistNew(token)
	return token
}

// CreateBackupDownload mints the short token an admin puts in a backup
// download URL instead of the account JWT. It is bound to one backup id and
// carries ScopeBackupDownload, which only that download route accepts.
func (s *StreamTokenStore) CreateBackupDownload(userID shared.UserID, backupID string, ttl time.Duration) string {
	if ttl <= 0 {
		ttl = s.ttl
	}
	token := uuid.New().String()
	s.issuedAt.Store(token, s.now())
	s.tokens.Store(token, &shared.StreamToken{
		Token:       token,
		UserID:      userID,
		MediaItemID: shared.MediaItemID(backupID),
		Scope:       shared.ScopeBackupDownload,
		ExpiresAt:   s.now().Add(ttl),
		TTL:         ttl,
	})
	s.persistNew(token)
	return token
}

// Validate checks the token, verifies it matches the mediaItemID, and renews the TTL.
// If mediaItemID is empty, any media is accepted (used by middleware).
func (s *StreamTokenStore) Validate(token, mediaItemID string) (*shared.StreamToken, bool) {
	var st *shared.StreamToken
	if v, ok := s.tokens.Load(token); ok {
		st = v.(*shared.StreamToken)
	} else if loaded, ok := s.loadPersisted(token); ok {
		// Minted before a restart: the player that holds it keeps playing.
		st = loaded
	} else {
		return nil, false
	}
	if s.now().After(st.ExpiresAt) {
		s.tokens.Delete(token)
		s.persistedExp.Delete(token)
		return nil, false
	}
	if mediaItemID != "" && st.MediaItemID != shared.MediaItemID(mediaItemID) {
		return nil, false
	}
	// Slide by the token's own TTL, not the store default - otherwise a
	// long-lived cast token would be downgraded to the 5-minute window
	// on its first segment fetch.
	ttl := st.TTL
	if ttl <= 0 {
		ttl = s.ttl
	}
	// Copy-on-write the sliding TTL: never mutate the shared *StreamToken in
	// place. The per-segment hot path fires Validate concurrently for the same
	// token while cleanupLoop ranges over it, and st.ExpiresAt is a multi-word
	// time.Time - an in-place write races those reads (a torn read -race only
	// surfaces under concurrent same-token traffic). Store a fresh pointer so
	// every reader sees an immutable snapshot; the loaded st is never written,
	// so copying it here is safe too.
	renewed := *st
	slidExpiry := s.now().Add(ttl)
	// Cap the slide at IssuedAt + maxAbsoluteStreamTokenTTL: a token renewed
	// forever by steady traffic must still die eventually. Once the absolute
	// deadline itself is in the past, ExpiresAt is set to it (not extended),
	// so the very next Validate call hits the expiry check above and rejects.
	if issued, ok := s.issuedAt.Load(token); ok {
		if absolute := issued.(time.Time).Add(maxAbsoluteStreamTokenTTL); absolute.Before(slidExpiry) {
			slidExpiry = absolute
		}
	}
	renewed.ExpiresAt = slidExpiry
	s.tokens.Store(token, &renewed)
	s.persistSlide(token, slidExpiry)
	return &renewed, true
}

// ValidateAny checks the token without media scope (middleware use).
func (s *StreamTokenStore) ValidateAny(token string) (*shared.StreamToken, bool) {
	return s.Validate(token, "")
}

func (s *StreamTokenStore) Revoke(token string) {
	s.tokens.Delete(token)
	s.issuedAt.Delete(token)
	s.persistedExp.Delete(token)
	s.forgetDurably(func(ctx context.Context) error { return s.repo.DeleteStreamToken(ctx, hashToken(token)) })
}

// RevokeByUser deletes every stream token minted for userID and returns how
// many were removed. Called on a security event (ban, password reset, logout,
// profile switch) so a stale long-lived cast/native token (up to 12h) is cut
// immediately instead of surviving until expiry - the JWT epoch bump alone
// left that gap, since opaque stream tokens are not epoch-guarded.
//
// Cast tokens (CreateCast) DO carry the minting user's UserID and are covered
// here like any other token. Syncplay guest tokens (CreateGuestToken) are the
// one deliberate exception: they are minted for an UNAUTHENTICATED "watch
// together" participant who has no account at all (Scope
// shared.ScopeSyncplayGuest, UserID always empty), so there is no user
// identity to key a revocation on in the first place - a guest session is
// bounded by its own short TTL and by its single-media-item scope
// (requireDownloadTokenBinding refuses it outside streaming), not by any
// user's login/ban/logout lifecycle. This is out of scope by construction,
// not an oversight: RevokeByUser("") is a deliberate no-op (see below) and a
// guest token's UserID is always "".
func (s *StreamTokenStore) RevokeByUser(userID string) int {
	if userID == "" {
		return 0
	}
	revoked := 0
	s.tokens.Range(func(key, value any) bool {
		st, ok := value.(*shared.StreamToken)
		if ok && string(st.UserID) == userID {
			s.tokens.Delete(key)
			s.issuedAt.Delete(key)
			s.persistedExp.Delete(key)
			revoked++
		}
		return true
	})
	// Also the tokens not loaded since the last restart.
	s.forgetDurably(func(ctx context.Context) error { return s.repo.DeleteStreamTokensByUser(ctx, shared.UserID(userID)) })
	return revoked
}

// RevokeByRoom drops every guest token minted by one syncplay room. It is the
// counterpart of RevokeByUser for the one token kind that has no user: closing
// the room is the host's only way to withdraw access a shared code handed out.
func (s *StreamTokenStore) RevokeByRoom(roomID string) int {
	if roomID == "" {
		return 0
	}
	revoked := 0
	s.tokens.Range(func(key, value any) bool {
		st, ok := value.(*shared.StreamToken)
		if ok && st.RoomID == roomID {
			s.tokens.Delete(key)
			s.issuedAt.Delete(key)
			s.persistedExp.Delete(key)
			revoked++
		}
		return true
	})
	s.forgetDurably(func(ctx context.Context) error { return s.repo.DeleteStreamTokensByRoom(ctx, roomID) })
	return revoked
}

func (s *StreamTokenStore) Stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *StreamTokenStore) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			bgloop.Tick("media.streamTokenCleanup", func() { s.purgeExpired(s.now()) })
		}
	}
}

// purgeExpired deletes every token whose ExpiresAt is before now. Extracted
// from cleanupLoop so it can be exercised directly in tests without waiting
// on the ticker.
func (s *StreamTokenStore) purgeExpired(now time.Time) {
	s.tokens.Range(func(key, value any) bool {
		st := value.(*shared.StreamToken)
		if now.After(st.ExpiresAt) {
			s.tokens.Delete(key)
			s.issuedAt.Delete(key)
			s.persistedExp.Delete(key)
		}
		return true
	})
	s.misses.Range(func(key, value any) bool {
		if now.After(value.(time.Time)) {
			s.misses.Delete(key)
			s.missCount.Add(-1)
		}
		return true
	})
	if s.repo != nil {
		ctx, cancel := s.persistCtx()
		defer cancel()
		if err := s.repo.DeleteExpiredStreamTokens(ctx, now); err != nil {
			slog.Debug("stream token: expired rows not purged", slog.Any("err", err))
		}
	}
}

// PurgeExpiredForTest exposes purgeExpired to tests (external package) so the
// cleanup logic can be verified deterministically at a given instant, without
// waiting on the cleanupLoop ticker.
func (s *StreamTokenStore) PurgeExpiredForTest(now time.Time) {
	s.purgeExpired(now)
}
