package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// captureDownloadLogs swaps the default logger for the duration of a test and
// returns the decoded lines. The download logging deliberately uses the package
// default rather than an injected logger, to match the rest of this file's
// handlers - so the test has to swap the default too.
func captureDownloadLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return func() []map[string]any {
		var out []map[string]any
		for raw := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
			if raw == "" {
				continue
			}
			var line map[string]any
			if err := json.Unmarshal([]byte(raw), &line); err != nil {
				t.Fatalf("log line is not JSON: %q (%v)", raw, err)
			}
			out = append(out, line)
		}
		return out
	}
}

func findLogLine(lines []map[string]any, msg string) map[string]any {
	for _, line := range lines {
		if line["msg"] == msg {
			return line
		}
	}
	return nil
}

// Every refusal on the download route used to set a span status and nothing
// else. A user reporting "the download does nothing" then left the operator
// with an empty journal - the failure mode that made the 2026-07-29 lockout
// take a day to diagnose. Each refusal must name itself.
func TestDownloadHandler_RefusalsAreLogged(t *testing.T) {
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/nonexistent"}}

	cases := []struct {
		name       string
		wantReason string
		token      *shared.StreamToken
		getter     *fakeMediaItemGetter
		access     LibraryAccessChecker
	}{
		{
			name:       "syncplay guest",
			wantReason: "syncplay_guest_token",
			token:      &shared.StreamToken{Token: "tok", UserID: "u1", MediaItemID: "m1", Scope: shared.ScopeSyncplayGuest},
			getter:     &fakeMediaItemGetter{item: item},
			access:     allowAll(),
		},
		{
			name:       "stream token bound to another item",
			wantReason: "stream_token_media_mismatch",
			token:      &shared.StreamToken{Token: "tok", UserID: "u1", MediaItemID: "m2"},
			getter:     &fakeMediaItemGetter{item: item},
			access:     allowAll(),
		},
		{
			name:       "unknown media item",
			wantReason: "media_item_lookup_failed",
			getter:     &fakeMediaItemGetter{},
			access:     allowAll(),
		},
		{
			// Answers 404 on purpose (an existence oracle would let a profile
			// enumerate other libraries), which is exactly why the LOG has to
			// carry the real reason.
			name:       "library access denied",
			wantReason: "library_access_denied",
			getter:     &fakeMediaItemGetter{item: item},
			access:     denyAll(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureDownloadLogs(t)

			h := downloadHandler(tc.getter, "http://localhost", tc.access, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
			req.SetPathValue("id", "m1")
			if tc.token != nil {
				req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, tc.token))
			}
			req = withHTTPClaims(req, testClaims(false))
			h.ServeHTTP(httptest.NewRecorder(), req)

			line := findLogLine(logs(), "download refused")
			if line == nil {
				t.Fatalf("no %q line was written - the refusal is invisible to an operator", "download refused")
			}
			if line["reason"] != tc.wantReason {
				t.Errorf("reason = %v, want %q", line["reason"], tc.wantReason)
			}
			if line["media_id"] != "m1" {
				t.Errorf("media_id = %v, want m1", line["media_id"])
			}
			if line["level"] != "WARN" {
				t.Errorf("level = %v, want WARN - a refused download is never routine", line["level"])
			}
		})
	}
}

// The success path needs a line too. Without one, "the client says nothing
// happened" and "the server never heard from the client" are indistinguishable
// in the journal, which is precisely the question an operator has to answer
// first. The source size rides along because a phone refusing a 19 GB remux is
// not a client bug.
func TestDownloadHandler_ServedDownloadIsLogged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	logs := captureDownloadLogs(t)

	h := downloadHandler(
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: path}}},
		"http://localhost", allowAll(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Range", "bytes=0-3")
	req = withHTTPClaims(req, testClaims(false))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent && rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 or 206 (body=%s)", rec.Code, rec.Body.String())
	}

	line := findLogLine(logs(), "download served")
	if line == nil {
		t.Fatal("a served download wrote no log line at all")
	}
	if line["media_id"] != "m1" {
		t.Errorf("media_id = %v, want m1", line["media_id"])
	}
	if got, want := line["bytes"], float64(10); got != want {
		t.Errorf("bytes = %v, want %v (the SOURCE size, not the bytes on the wire)", got, want)
	}
	if line["range"] != "bytes=0-3" {
		t.Errorf("range = %v, want the request's Range header - a resumed transfer must be tellable from a fresh one", line["range"])
	}
	if line["filename"] != "film.mkv" {
		t.Errorf("filename = %v, want film.mkv", line["filename"])
	}
}
