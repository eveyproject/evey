package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/music"
	musicfakes "github.com/evey-project/evey/internal/domain/music/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// id3WithPicture forges the smallest ID3v2.3 tag carrying one APIC frame -
// what the tag reader needs to hand back an embedded cover.
func id3WithPicture(mime string, pic []byte) []byte {
	body := []byte{0x00}
	body = append(body, mime...)
	body = append(body, 0x00, 0x03, 0x00)
	body = append(body, pic...)
	frame := append([]byte("APIC"), byte(len(body)>>24), byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	frame = append(frame, 0x00, 0x00)
	frame = append(frame, body...)
	n := len(frame)
	out := append([]byte("ID3"), 0x03, 0x00, 0x00, byte(n>>21&0x7F), byte(n>>14&0x7F), byte(n>>7&0x7F), byte(n&0x7F))
	return append(out, frame...)
}

// The embedded cover is served with the tag's MIME type; ?w= produces a
// WebP variant through the resizer once (the second request is a cache hit),
// and a track whose file carries no picture is a plain 404.
func TestGetTrackArt_ServesEmbeddedCoverAndVariant(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user-1"})

	repo := musicfakes.NewFakeMusicTrackRepository()
	scanner := musicfakes.NewFakeMusicScanner()
	png := []byte{0x89, 'P', 'N', 'G', 'f', 'a', 'k', 'e'}
	scanner.FileData["/music/a/01.mp3"] = id3WithPicture("image/png", png)
	scanner.FileData["/music/a/02.mp3"] = []byte("ID3\x03\x00\x00\x00\x00\x00\x00")
	ctx := context.Background()
	if _, err := repo.Upsert(ctx, music.Track{ID: "trk-art", LibraryID: shared.LibraryID("lib1"), Title: "One", FilePath: "/music/a/01.mp3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, music.Track{ID: "trk-bare", LibraryID: shared.LibraryID("lib1"), Title: "Two", FilePath: "/music/a/02.mp3"}); err != nil {
		t.Fatal(err)
	}
	svc := appmusic.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	conv := &fakeImageConverter{}
	cacheDir := t.TempDir()
	mux := http.NewServeMux()
	registerMusicRoutes(mux, svc, tokenGen, allowAll(), nil, ratelimit.New(10000), nil, conv, nil, withTrackArtCacheDir(cacheDir))

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	w := get("/api/v1/music/tracks/trk-art/art")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || w.Body.String() != string(png) {
		t.Fatalf("original: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.Bytes())
	}
	if w.Header().Get("Cache-Control") == "" {
		t.Error("art is cacheable")
	}

	w = get("/api/v1/music/tracks/trk-art/art?w=48")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/webp" || w.Body.String() != "RIFF-webp-resized" {
		t.Fatalf("variant: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "trk-art.png")); err != nil {
		t.Errorf("the tag bytes are written once as the resizer's source: %v", err)
	}
	get("/api/v1/music/tracks/trk-art/art?w=48")
	if conv.resizeCalled != 1 {
		t.Errorf("resizer called %d times, want 1 (second request is a cache hit)", conv.resizeCalled)
	}

	w = get("/api/v1/music/tracks/trk-bare/art")
	if w.Code != http.StatusNotFound {
		t.Errorf("no picture in the tag: %d, want 404", w.Code)
	}
}
