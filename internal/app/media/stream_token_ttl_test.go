package media_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Every mint path guards its TTL argument with `if ttl <= 0 { ttl = s.ttl }`.
// Mutation (2026-08-01) showed not one of those guards was pinned: the whole
// family survived, so the requested lifetime could be silently replaced by the
// default and every test stayed green.
//
// That is not hypothetical. CastStreamTokenTTL exists precisely because a cast
// receiver buffers minutes ahead then goes quiet, and a 5-minute token dies
// mid-film - the bug the 12h window was introduced to fix. Ignoring an explicit
// TTL brings it straight back.
func TestStreamTokenStore_ExplicitTTLIsHonoured(t *testing.T) {
	base := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	const defaultTTL = 5 * time.Minute

	cases := []struct {
		name string
		ttl  time.Duration
		want time.Duration
		mint func(s *media.StreamTokenStore, ttl time.Duration) string
	}{
		{
			name: "cast token keeps its long window",
			ttl:  media.CastStreamTokenTTL,
			want: media.CastStreamTokenTTL,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateCast("u1", "p1", "m1", ttl)
			},
		},
		{
			name: "cast token with 0 falls back to the default",
			ttl:  0,
			want: defaultTTL,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateCast("u1", "p1", "m1", ttl)
			},
		},
		{
			name: "explicit TTL is honoured on the generic path",
			ttl:  90 * time.Minute,
			want: 90 * time.Minute,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateWithTTL("u1", "p1", "m1", ttl)
			},
		},
		{
			name: "generic path with 0 falls back to the default",
			ttl:  0,
			want: defaultTTL,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateWithTTL("u1", "p1", "m1", ttl)
			},
		},
		{
			name: "syncplay guest token keeps its explicit window",
			ttl:  30 * time.Minute,
			want: 30 * time.Minute,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateGuestToken("m1", "room-1", ttl)
			},
		},
		// The two mint paths the table missed: a zero TTL is "use the default",
		// never a token born expired (mutation 2026-09-28).
		{
			name: "syncplay guest token with 0 falls back to the default",
			ttl:  0,
			want: defaultTTL,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateGuestToken("m1", "room-1", ttl)
			},
		},
		{
			name: "backup download token with 0 falls back to the default",
			ttl:  0,
			want: defaultTTL,
			mint: func(s *media.StreamTokenStore, ttl time.Duration) string {
				return s.CreateBackupDownload("u1", "bk-1", ttl)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := media.NewStreamTokenStoreForTest(defaultTTL, func() time.Time { return base })
			defer s.Stop()

			tok := tc.mint(s, tc.ttl)
			st, ok := s.ValidateAny(tok)
			if !ok {
				t.Fatal("the token was refused right after being minted")
			}

			if got := st.ExpiresAt.Sub(base); got != tc.want {
				t.Fatalf("token expires in %v, want %v: a lifetime asked for explicitly "+
					"was replaced by another. On a cast this is the token dying mid-film.",
					got, tc.want)
			}
			// TTL is the sliding renewal window Validate reuses on every segment
			// fetch; if it disagrees with ExpiresAt, the first fetch silently
			// downgrades the token.
			if st.TTL != tc.want {
				t.Fatalf("stored sliding window is %v, want %v: the next segment fetch "+
					"would renew the token to the wrong lifetime", st.TTL, tc.want)
			}
		})
	}
}

// A guest token must stay confined to the room's media item and carry the guest
// scope, so a non-streaming endpoint can refuse it.
func TestStreamTokenStore_GuestTokenStaysScoped(t *testing.T) {
	base := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	s := media.NewStreamTokenStoreForTest(5*time.Minute, func() time.Time { return base })
	defer s.Stop()

	tok := s.CreateGuestToken("m1", "room-1", 30*time.Minute)

	if _, ok := s.Validate(tok, "m1"); !ok {
		t.Fatal("the guest token was refused on its own media item")
	}
	if _, ok := s.Validate(tok, "m2"); ok {
		t.Fatal("the guest token was accepted on another media item: a watch-together " +
			"guest would reach content the room never shared")
	}
	st, _ := s.ValidateAny(tok)
	if st.Scope != shared.ScopeSyncplayGuest {
		t.Fatalf("guest token scope is %q, want %q: without the marker no endpoint "+
			"can tell a guest apart from a full viewer", st.Scope, shared.ScopeSyncplayGuest)
	}
}
