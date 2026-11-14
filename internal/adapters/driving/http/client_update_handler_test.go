package httpapi

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testClientUpdateManifest = `{"id":"0b5b7a32-6f0c-4c58-9d0e-0d0f6a6f0c11","runtimeVersion":"tv-1"}`
	testClientUpdateSig      = `sig="c2lnbmVk", keyid="main"`
	testClientUpdateAsset    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.hbc"
)

func newClientUpdateTestServer(t *testing.T, withSig bool) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	bundle := filepath.Join(dir, "tv", "android", "tv-1")
	if err := os.MkdirAll(filepath.Join(bundle, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(bundle, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifest.json", testClientUpdateManifest)
	if withSig {
		write("manifest.sig", testClientUpdateSig+"\n")
	}
	write(filepath.Join("assets", testClientUpdateAsset), "bytecode")
	// Sits next to the bundle: an asset request must never reach it.
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerClientUpdateRoutes(mux, dir)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func getClientUpdateManifest(t *testing.T, srv *httptest.Server, app, platform, runtime string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/client-updates/"+app+"/manifest", nil)
	if err != nil {
		t.Fatal(err)
	}
	if platform != "" {
		req.Header.Set("expo-platform", platform)
	}
	if runtime != "" {
		req.Header.Set("expo-runtime-version", runtime)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestClientUpdateManifest_ServesSignedBytesUntouched(t *testing.T) {
	srv := newClientUpdateTestServer(t, true)
	resp := getClientUpdateManifest(t, srv, "tv", "android", "tv-1")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("expo-protocol-version"); got != "1" {
		t.Errorf("expo-protocol-version = %q, want 1", got)
	}
	if got := resp.Header.Get("expo-sfv-version"); got != "0" {
		t.Errorf("expo-sfv-version = %q, want 0", got)
	}
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("content-type = %q (%v), want multipart/mixed", resp.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(resp.Body, params["boundary"])

	part, err := reader.NextPart()
	if err != nil {
		t.Fatalf("manifest part: %v", err)
	}
	if part.FormName() != "manifest" {
		t.Errorf("first part = %q, want manifest", part.FormName())
	}
	if got := part.Header.Get("expo-signature"); got != testClientUpdateSig {
		t.Errorf("expo-signature = %q, want %q", got, testClientUpdateSig)
	}
	body, _ := io.ReadAll(part)
	// The signature covers these exact bytes: one byte of difference and
	// every app refuses the update.
	if string(body) != testClientUpdateManifest {
		t.Errorf("manifest body = %q, want the file as written", body)
	}

	part, err = reader.NextPart()
	if err != nil || part.FormName() != "extensions" {
		t.Fatalf("second part = %v (%v), want extensions", part, err)
	}
}

func TestClientUpdateManifest_NothingToOffer(t *testing.T) {
	srv := newClientUpdateTestServer(t, true)
	cases := []struct{ name, app, platform, runtime string }{
		{"other runtime", "tv", "android", "tv-2"},
		{"other platform", "tv", "ios", "tv-1"},
		{"app without a bundle", "mobile", "android", "tv-1"},
		{"unknown app", "desktop", "android", "tv-1"},
		{"no runtime header", "tv", "android", ""},
		{"runtime that walks up", "tv", "android", "../android/tv-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := getClientUpdateManifest(t, srv, tc.app, tc.platform, tc.runtime)
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", resp.StatusCode)
			}
			// The app reads these before deciding the answer means "no update".
			if resp.Header.Get("expo-protocol-version") != "1" {
				t.Errorf("expo-protocol-version missing on 204")
			}
		})
	}
}

func TestClientUpdateManifest_UnsignedBundleIsNotOffered(t *testing.T) {
	srv := newClientUpdateTestServer(t, false)
	resp := getClientUpdateManifest(t, srv, "tv", "android", "tv-1")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 for a bundle without signature", resp.StatusCode)
	}
}

func TestClientUpdateManifest_NoDirectoryConfigured(t *testing.T) {
	mux := http.NewServeMux()
	registerClientUpdateRoutes(mux, "")
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	resp := getClientUpdateManifest(t, srv, "tv", "android", "tv-1")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestClientUpdateAsset(t *testing.T) {
	srv := newClientUpdateTestServer(t, true)
	base := srv.URL + "/api/v1/client-updates/tv/assets/"

	resp, err := http.Get(base + "android/tv-1/" + testClientUpdateAsset)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "bytecode" {
		t.Fatalf("asset = %d %q, want 200 bytecode", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/javascript" {
		t.Errorf("content-type = %q, want application/javascript", got)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("cache-control = %q, want immutable", got)
	}

	for _, path := range []string{
		"android/tv-1/manifest.json",
		"android/tv-1/..%2F..%2F..%2F..%2Fsecret",
		"android/tv-2/" + testClientUpdateAsset,
		"windows/tv-1/" + testClientUpdateAsset,
	} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}
