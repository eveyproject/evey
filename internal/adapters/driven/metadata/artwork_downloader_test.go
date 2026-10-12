package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/pkg/ssrf"
)

func TestNewArtworkHTTPDownloader_DefaultClient(t *testing.T) {
	d := NewArtworkHTTPDownloader(nil)
	assert.NotNil(t, d.httpClient)
}

func TestArtworkDownloader_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("fake-image-data"))
	}))
	defer srv.Close()

	d := NewArtworkHTTPDownloader(srv.Client())
	dest := filepath.Join(t.TempDir(), "test.jpg")
	err := d.Download(context.Background(), srv.URL+"/poster.jpg", dest)
	require.NoError(t, err)

	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "fake-image-data", string(data))
}

func TestArtworkDownloader_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	d := NewArtworkHTTPDownloader(srv.Client())
	dest := filepath.Join(t.TempDir(), "test.jpg")
	err := d.Download(context.Background(), srv.URL+"/missing.jpg", dest)
	assert.ErrorContains(t, err, "status 404")
}

func TestArtworkDownloader_WrongContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>not an image</html>"))
	}))
	defer srv.Close()

	d := NewArtworkHTTPDownloader(srv.Client())
	dest := filepath.Join(t.TempDir(), "test.jpg")
	err := d.Download(context.Background(), srv.URL+"/page.html", dest)
	assert.ErrorContains(t, err, "unexpected content-type")
}

func TestArtworkDownloader_InvalidDestPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	d := NewArtworkHTTPDownloader(srv.Client())
	err := d.Download(context.Background(), srv.URL+"/img.png", "/nonexistent/dir/file.png")
	assert.ErrorContains(t, err, "create file")
}

// TestArtworkDownloader_SSRFBlocksLoopback verifies that the default client
// (nil passed to NewArtworkHTTPDownloader) refuses to connect to 127.0.0.1,
// proving the SSRF-safe client is wired in.
func TestArtworkDownloader_SSRFBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	d := NewArtworkHTTPDownloader(nil)
	dest := filepath.Join(t.TempDir(), "blocked.png")
	err := d.Download(context.Background(), srv.URL+"/img.png", dest)
	require.Error(t, err)
	if !errors.Is(err, ssrf.ErrBlockedPrivateIP) {
		t.Fatalf("expected ErrBlockedPrivateIP in chain, got %v", err)
	}
}
