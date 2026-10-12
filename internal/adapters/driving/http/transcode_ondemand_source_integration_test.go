package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	mkvgoadapter "github.com/eveyproject/evey/internal/adapters/driven/mkvgo"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// TestOnDemandSourceReplacedIntegration plays a media through the real mkvgo
// packager over HTTP and replaces its file mid-playback: the segment URL of
// the old playlist answers 410, the playlist fetched again names the new file
// and its segment URL serves the new file's bytes. Run twice: a replacement
// that changes the file's size (a new plan cache key), and one that keeps its
// size and mtime (same key: the cached plan is refused by mkvgo, rebuilt and
// the request replayed, which then finds another source). Opt-in:
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driving/http/ -run SourceReplacedIntegration -v
func TestOnDemandSourceReplacedIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	for _, sameStamp := range []bool{false, true} {
		name := "new size"
		if sameStamp {
			name = "same size and mtime"
		}
		t.Run(name, func(t *testing.T) { sourceReplacedOverHTTP(t, sameStamp) })
	}
}

func sourceReplacedOverHTTP(t *testing.T, sameStamp bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dir := t.TempDir()
	src := filepath.Join(dir, "movie.mkv")
	replacement := filepath.Join(dir, "replacement.mkv")
	gen := func(path, pattern string) {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
			"-f", "lavfi", "-i", pattern+"=size=320x180:rate=24",
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
			"-t", "24",
			"-c:v", "libx264", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "128k",
			path)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
		}
	}
	gen(src, "testsrc2")
	gen(replacement, "testsrc")
	if sameStamp {
		// Pad the smaller file to the other's size with an EBML Void element
		// (skipped by every reader) and give the replacement the source's
		// mtime: the plan cache key (path, size, mtime) is the same before and
		// after.
		a, b := fileSize(t, src), fileSize(t, replacement)
		pad := func(path string, n int64) {
			if n < 9 {
				t.Skipf("sizes %d apart, too close for a Void element", n)
			}
			// ID 0xEC, then the body size as an 8-byte VINT (0x01 + 7 bytes).
			void := make([]byte, n)
			void[0], void[1] = 0xEC, 0x01
			body := uint64(n - 9)
			for i := range 7 {
				void[8-i] = byte(body >> (8 * i))
			}
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(void); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
		}
		if a < b {
			pad(src, b-a)
		} else if b < a {
			pad(replacement, a-b)
		}
		fi, err := os.Stat(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(replacement, fi.ModTime(), fi.ModTime()); err != nil {
			t.Fatal(err)
		}
	}

	// What the replacement serves, from a packager of its own.
	opts := apptranscoding.OnDemandOptions{SegmentMs: onDemandSegmentMs, URLPrefix: onDemandPrefix}
	ref := mkvgoadapter.NewOnDemandPackager()
	defer ref.Close()
	refPlan, err := ref.Plan(ctx, replacement, opts)
	if err != nil {
		t.Fatalf("reference Plan: %v", err)
	}
	wantSeg, _, err := refPlan.Resource(ctx, "seg00001.m4s")
	if err != nil {
		t.Fatalf("reference segment: %v", err)
	}

	// The server: every request asks the packager for the file's plan, as
	// serveOnDemandResource does, and serves through servePlanResourceData.
	pkg := mkvgoadapter.NewOnDemandPackager()
	defer pkg.Close()
	h := &hlsVODServe{onDemand: pkg}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plan, err := pkg.Plan(r.Context(), src, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/"), onDemandPrefix)
		h.servePlanResourceData(w, r, "m1", plan, nil, name, trace.SpanFromContext(r.Context()))
	}))
	defer srv.Close()

	get := func(uri string) (int, []byte) {
		t.Helper()
		res, err := http.Get(srv.URL + "/" + uri)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, body
	}
	firstSegURI := func() string {
		t.Helper()
		code, pl := get("od_playlist.m3u8")
		if code != http.StatusOK {
			t.Fatalf("playlist: %d %s", code, pl)
		}
		for ln := range strings.SplitSeq(string(pl), "\n") {
			if strings.HasPrefix(ln, onDemandPrefix+"seg00001.m4s") {
				if !strings.Contains(ln, "&"+planSourceParam+"=") {
					t.Fatalf("segment URI without the source: %q", ln)
				}
				return ln
			}
		}
		t.Fatalf("no seg00001 in the playlist:\n%s", pl)
		return ""
	}

	oldURI := firstSegURI()
	code, before := get(oldURI)
	if code != http.StatusOK {
		t.Fatalf("segment before the replacement: %d", code)
	}
	if bytes.Equal(before, wantSeg) {
		t.Fatal("the two sources serve the same segment: the test proves nothing")
	}

	if err := os.Rename(replacement, src); err != nil {
		t.Fatalf("replace the source: %v", err)
	}

	if gone, body := get(oldURI); gone != http.StatusGone {
		t.Fatalf("old segment URL after the replacement: %d (%d bytes), want 410", gone, len(body))
	}
	newURI := firstSegURI()
	if newURI == oldURI {
		t.Fatalf("the playlist still names the old source: %q", newURI)
	}
	code, got := get(newURI)
	if code != http.StatusOK {
		t.Fatalf("new segment URL: %d %s", code, got)
	}
	if !bytes.Equal(got, wantSeg) {
		t.Errorf("new segment URL: %d bytes, not the new file's segment (%d bytes)", len(got), len(wantSeg))
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}
