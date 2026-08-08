package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// downloadSample must verify the SHA256 pin: a matching hash writes the file
// atomically, a mismatch is rejected WITHOUT leaving a bogus clip at dst (which
// the bench would otherwise calibrate against).
func TestDownloadSample_VerifiesSHA256Pin(t *testing.T) {
	payload := []byte("pretend-this-is-a-4k-hevc-reference-clip")
	sum := sha256.Sum256(payload)
	wantSHA := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := NewBenchmarkRunner(BenchmarkOpts{}, nil)

	// Correct pin → file written with the served bytes.
	good := filepath.Join(t.TempDir(), "clip.hevc")
	if err := r.downloadSample(srv.URL, wantSHA, good); err != nil {
		t.Fatalf("downloadSample (matching sha): %v", err)
	}
	got, err := os.ReadFile(good)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("downloaded content mismatch: err=%v got=%q", err, got)
	}

	// Wrong pin → error, and NO file left at dst.
	bad := filepath.Join(t.TempDir(), "clip.hevc")
	if err := r.downloadSample(srv.URL, "00bad00", bad); err == nil {
		t.Fatal("downloadSample accepted a mismatched sha256 pin")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatalf("mismatched download must not leave a file at dst, stat err=%v", err)
	}
}

// resolveBenchSample resolves in priority order: operator override > cached
// default > pinned download > synthetic generate > lavfi fallback. Each step is
// only reached when the prior one is absent.
func TestResolveBenchSample_Cascade(t *testing.T) {
	r := NewBenchmarkRunner(BenchmarkOpts{}, nil)
	dir := t.TempDir()
	mustWrite := func(p string) string {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	noGen := func(string) error { t.Fatal("generate must not run when an earlier step resolves"); return nil }

	// 1. operator override present → used verbatim.
	override := mustWrite(filepath.Join(dir, "override.hevc"))
	got, fb := r.resolveBenchSample(context.Background(), "T", benchSampleSource{overridePath: override}, filepath.Join(dir, "d1.hevc"), noGen)
	if got != override || fb {
		t.Fatalf("override step: got %q fallback=%v", got, fb)
	}

	// 2. override missing, default cached on disk → returns the cache.
	cached := mustWrite(filepath.Join(dir, "d2.hevc"))
	got, fb = r.resolveBenchSample(context.Background(), "T", benchSampleSource{overridePath: filepath.Join(dir, "nope")}, cached, noGen)
	if got != cached || fb {
		t.Fatalf("cached step: got %q fallback=%v", got, fb)
	}

	// 3. nothing cached, pinned download succeeds → returns the downloaded path.
	payload := []byte("downloaded-clip")
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer srv.Close()
	dlDst := filepath.Join(dir, "d3.hevc")
	got, fb = r.resolveBenchSample(context.Background(), "T", benchSampleSource{url: srv.URL, sha256: hex.EncodeToString(sum[:])}, dlDst, noGen)
	if got != dlDst || fb {
		t.Fatalf("download step: got %q fallback=%v", got, fb)
	}

	// 4. no override/cache/url → synthetic generate.
	genDst := filepath.Join(dir, "d4.hevc")
	got, fb = r.resolveBenchSample(context.Background(), "T", benchSampleSource{}, genDst, func(dst string) error { return os.WriteFile(dst, []byte("g"), 0o644) })
	if got != genDst || fb {
		t.Fatalf("generate step: got %q fallback=%v", got, fb)
	}

	// 5. generate fails → lavfi fallback (path empty, fallback true).
	got, fb = r.resolveBenchSample(context.Background(), "T", benchSampleSource{}, filepath.Join(dir, "d5.hevc"), func(string) error { return errors.New("boom") })
	if got != "" || !fb {
		t.Fatalf("fallback step: got %q fallback=%v", got, fb)
	}
}

// synthBenchSource locks the synthetic recipe shape: testsrc2 (real motion) +
// gblur (soften synthetic edges toward film) + light grain (decode non-trivial
// without the old noise=20 encode pathology), at the requested pixel format.
func TestSynthBenchSource_Recipe(t *testing.T) {
	hdr := synthBenchSource("yuv420p10le")
	for _, want := range []string{"testsrc2", "gblur=", "noise=alls=4", "yuv420p10le"} {
		if !strings.Contains(hdr, want) {
			t.Errorf("synthBenchSource(10le) missing %q in %q", want, hdr)
		}
	}
	if strings.Contains(hdr, "noise=alls=20") {
		t.Error("synthBenchSource still uses the pathological noise=alls=20")
	}
	if sdr := synthBenchSource("yuv420p"); !strings.Contains(sdr, "format=yuv420p") {
		t.Errorf("synthBenchSource(8) wrong pix fmt: %q", sdr)
	}
}
