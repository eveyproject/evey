package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// Every URL a client really builds with the account JWT in ?token= is
// accepted. The comment names the client file:line that builds it (paths
// relative to frontend/), so an entry nobody builds any more can be found and
// dropped.
func TestJWTQueryAllowed_AcceptsTheURLsClientsBuild(t *testing.T) {
	cases := []struct{ method, url string }{
		// apps/web/src/features/music/GlobalAudioPlayer.tsx:203
		// apps/web/src/routes/_auth/syncplay.lazy.tsx:188
		// apps/mobile/src/components/MusicMiniPlayer.tsx:80
		// apps/mobile/src/features/syncplay/ListenTogetherRoom.tsx:59
		// apps/tv/src/components/TVMusicPlayer.tsx:108
		// apps/tv-web/src/music/MusicBar.tsx:53
		{http.MethodGet, "/api/v1/music/tracks/t1/stream"},
		// apps/web/src/features/music/GlobalAudioPlayer.tsx:15
		// apps/web/src/features/music/TrackArt.tsx:11
		// apps/web/src/routes/_auth/syncplay.lazy.tsx:195
		// apps/mobile/src/components/MusicMiniPlayer.tsx:307
		// apps/tv/src/components/TVMusicPlayer.tsx:161
		// and the coverArtUrl the server hands out for embedded art
		// (internal/app/music/command_service.go:836).
		{http.MethodGet, "/api/v1/music/tracks/t1/art?w=96"},
		// The coverArtUrl of an album (internal/app/music/command_service.go:321,
		// album_art_folder.go:67) with the JWT appended:
		// apps/web/src/lib/coverArtSrc.ts:9 from home.lazy.tsx:53,
		// AlbumHeader.tsx:25, AlbumsTab.tsx:71, ArtistsTab.tsx:85,
		// RoomBuilder.tsx:240, DashboardNowPlaying.tsx:51
		// apps/mobile/app/(auth)/(tabs)/index.tsx:74
		// apps/tv/app/(auth)/(tabs)/index.tsx:665
		// apps/tv-web/src/screens/Home.tsx:291 (lib/media.ts:35)
		// apps/tv-web/src/components/SearchPanel.tsx:25
		// apps/tv-web/src/screens/Music.tsx:29
		// apps/tv-web/src/music/MusicBar.tsx:54
		{http.MethodGet, "/api/v1/music/album-art/0123abcd?w=185"},
		// apps/web/src/routes/_auth/home.lazy.tsx:76
		// apps/web/src/routes/_auth/photos/index.lazy.tsx:262, 366, 469
		// apps/mobile/app/(auth)/(tabs)/index.tsx:151
		// apps/tv/app/(auth)/(tabs)/index.tsx:689
		// apps/tv-web/src/screens/Home.tsx:299 (lib/media.ts:35)
		// apps/tv-web/src/screens/Photos.tsx:35
		{http.MethodGet, "/api/v1/photos/p1/serve?w=400&fmt=webp"},
		{http.MethodGet, "/api/v1/photos/p1/serve"},
		{http.MethodHead, "/api/v1/photos/p1/serve"},
		// apps/mobile/src/components/layout/ProfileSwitcher.tsx:115, 219
		// apps/mobile/src/features/profile/ProfileManagerList.tsx:75
		// apps/mobile/app/(auth)/(tabs)/settings.tsx:253
		// apps/mobile/app/(auth)/admin/users.tsx:242
		// apps/mobile/app/profile-select.tsx:224
		// apps/tv-web/src/lib/media.ts:39
		{http.MethodGet, "/api/v1/profiles/pr1/avatar"},
		// apps/web/src/features/library/artworkUrl.ts:37 from MediaDetail.tsx:60-61,
		// MediaDetailSeries.tsx:30, 128
		// packages/client/src/hooks/useAuthenticatedImage.ts:27, through
		// apps/mobile/src/hooks/useAuthenticatedImage.ts:7, from
		// apps/mobile/src/components/MediaCard.tsx:61 and
		// apps/mobile/app/(auth)/history.tsx:56
		// apps/tv-web/src/lib/media.ts:11
		{http.MethodGet, "/api/v1/media/m1/artwork/poster?v=3"},
		{http.MethodGet, "/api/v1/media/m1/artwork/backdrop?w=1920"},
		// apps/web/src/features/admin/AdminTrickplayThumbnail.tsx:72, 83
		{http.MethodGet, "/api/v1/media/m1/snapshot?position_ms=60000"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.url, func(t *testing.T) {
			if !jwtQueryAllowed(httptest.NewRequest(tc.method, tc.url, nil)) {
				t.Fatalf("%s %s: a URL a client builds with the JWT must be accepted", tc.method, tc.url)
			}
		})
	}
}

// A read that returns account JSON, or a route only ever called with a header
// or a stream token, refuses a JWT in the URL. Every path below is a real
// route (proto http annotations or a register*Routes); the siblings of the
// accepted routes and the malformed look-alikes check the match is exact.
func TestJWTQueryAllowed_RefusesOtherReads(t *testing.T) {
	paths := []string{
		// Account JSON through the gateway (proto/evey/v1/*.proto).
		"/api/v1/users/me",
		"/api/v1/users",
		"/api/v1/users/u1/profiles",
		"/api/v1/api-keys",
		"/api/v1/activity/logins",
		"/api/v1/activity/history",
		"/api/v1/playback/history",
		"/api/v1/notifications/settings",
		"/api/v1/push/devices",
		"/api/v1/invites",
		"/api/v1/profiles/pr1/parental",
		"/api/v1/media",
		"/api/v1/photos/p1",
		"/api/v1/music/tracks/t1",
		// HTTP mux reads.
		"/api/v1/devices",
		"/api/v1/playback-settings",
		"/api/v1/plugins/pl1/config",
		"/api/v1/profiles/pr1/schedule",
		"/api/v1/media/m1/playback-info",
		"/api/v1/cast/devices",
		"/metrics",
		// Stream-token routes: the clients send an opaque token there.
		"/api/v1/media/m1/stream",
		"/api/v1/media/m1/hls/master.m3u8",
		"/api/v1/media/m1/subtitle/2",
		"/api/v1/media/m1/download",
		"/api/v1/media/m1/trickplay/trickplay_001.jpg",
		"/api/v1/music/tracks/t1/download",
		"/api/v1/playlists/pl1/download",
		// Look-alikes of the accepted shapes.
		"/api/v1/photos/p1/serve/extra",
		"/api/v1/photos//serve",
		"/api/v1/photos/p1/serve/",
		"/api/v1/profiles/../avatar",
		"/api/v1/media/m1/artwork",
		"/api/v1/media/m1/artwork/poster/x",
		"/api/v1/music/album-art/",
		"/v1/photos/p1/serve",
		"/prefix/api/v1/photos/p1/serve",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			if jwtQueryAllowed(httptest.NewRequest(http.MethodGet, p, nil)) {
				t.Fatalf("GET %s: a JWT in the URL must be refused here", p)
			}
		})
	}
}

// A JWT in the URL never writes, even on an accepted path.
func TestJWTQueryAllowed_RefusesWrites(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
		for _, p := range []string{"/api/v1/photos/p1/serve", "/api/v1/profiles/pr1/avatar", "/api/v1/users/me"} {
			t.Run(method+" "+p, func(t *testing.T) {
				if jwtQueryAllowed(httptest.NewRequest(method, p, nil)) {
					t.Fatalf("%s %s: a JWT in the URL must not write", method, p)
				}
			})
		}
	}
}

// /api/v1/admin/ refuses a JWT in the URL, including a path that would
// otherwise end like an accepted one. The backup download rides an opaque
// backup-download token (POST .../download-token, then ?token=), resolved
// before this rule is ever consulted.
func TestJWTQueryAllowed_RefusesAdmin(t *testing.T) {
	for _, p := range []string{
		"/api/v1/admin/backups/bk-1",
		"/api/v1/admin/users",
		"/api/v1/admin/logs/search",
		"/api/v1/admin/api/v1/photos/p1/serve",
	} {
		t.Run(p, func(t *testing.T) {
			if jwtQueryAllowed(httptest.NewRequest(http.MethodGet, p, nil)) {
				t.Fatalf("GET %s: the admin surface must refuse a JWT in the URL", p)
			}
		})
	}
}

// End to end through the token middleware and the real auth middleware: a
// valid JWT in the URL authenticates an accepted route, and leaves a refused
// one unauthenticated (401), with the token gone from the URL in both cases.
func TestJWTQuery_MiddlewareAuthenticatesOnlyAllowlistedRoutes(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	gen := auth.NewJWTGenerator(key, time.Minute)
	jwt, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "pr1"})
	if err != nil {
		t.Fatal(err)
	}
	authMw := httpAuthMiddleware(gen)
	var sawUser, sawToken string
	probe := func(w http.ResponseWriter, r *http.Request) {
		sawToken = r.URL.Query().Get("token")
		if c := HTTPClaimsFromContext(r.Context()); c != nil {
			sawUser = c.UserID
		}
		w.WriteHeader(http.StatusOK)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/photos/{id}/serve", authMw(probe))
	mux.HandleFunc("GET /api/v1/devices", authMw(probe))
	mux.HandleFunc("GET /api/v1/media/{id}/stream", httpStreamAuthMiddleware(gen)(probe))
	h := TokenRedactMiddleware(mux)

	cases := []struct {
		path, query string
		wantCode    int
	}{
		{"/api/v1/photos/p1/serve", "?w=400&token=", http.StatusOK},
		{"/api/v1/devices", "?token=", http.StatusUnauthorized},
		{"/api/v1/media/m1/stream", "?token=", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			sawUser, sawToken = "", ""
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path+tc.query+jwt, nil))
			if w.Code != tc.wantCode {
				t.Fatalf("GET %s: code %d, want %d", tc.path, w.Code, tc.wantCode)
			}
			if tc.wantCode == http.StatusOK && sawUser != "u1" {
				t.Fatalf("accepted route: claims user %q, want u1", sawUser)
			}
			if sawToken != "" {
				t.Fatal("the token must leave the URL")
			}
		})
	}
}
