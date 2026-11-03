package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

// sampleTestRunner is a runner whose "ffmpeg" is the given binary, with its
// samples under dir.
func sampleTestRunner(t *testing.T, dir, ffmpeg string) *BenchmarkRunner {
	t.Helper()
	return NewBenchmarkRunner(BenchmarkOpts{FfmpegPath: ffmpeg, SamplePath: filepath.Join(dir, "sample.hevc")}, slog.New(slog.DiscardHandler))
}

// fakeEncoder is an "ffmpeg" that writes payload to its output (the last
// argument) and exits with code.
func fakeEncoder(t *testing.T, dir, payload string, code int) string {
	t.Helper()
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nfor out; do :; done\nprintf '%s' '" + payload + "' > \"$out\"\nexit " + strconv.Itoa(code) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // G306: a test helper must be executable
		t.Fatal(err)
	}
	return bin
}

// A sample encode that fails leaves NOTHING at dst: ffmpeg opens its output
// before the first frame, and the 0-byte clip a crash left behind used to be
// found "cached", condemned and paid for again at every calibration.
func TestEncodeSample_FailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "sample-hdr10.hevc")

	crash := fakeEncoder(t, dir, "partial", 139)
	err := sampleTestRunner(t, dir, crash).encodeSample(dst, "t", 0, "-i", "x")
	if err == nil {
		t.Fatal("a crashed encode must be an error")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("a failed encode must leave no clip at dst, stat: %v", statErr)
	}

	mute := fakeEncoder(t, dir, "", 0)
	if err = sampleTestRunner(t, dir, mute).encodeSample(dst, "t", 0, "-i", "x"); err == nil {
		t.Fatal("a clean exit that wrote nothing must be an error, not a 0-byte clip")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("an empty encode must leave no clip at dst, stat: %v", statErr)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

// A clean encode lands at dst, whole, with no temp file left beside it.
func TestEncodeSample_SuccessRenamesIntoPlace(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "sample-hdr10.hevc")
	ok := fakeEncoder(t, dir, "clip-bytes", 0)
	if err := sampleTestRunner(t, dir, ok).encodeSample(dst, "t", 0, "-i", "x"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "clip-bytes" {
		t.Fatalf("dst must hold the encode: %q, %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

// The HDR sample is resolved once per bench: the software HDR ladder and the
// HW bridge both ask for it, and a generation that failed is not retried
// within the bench (a crashing x265 used to be paid twice per calibration).
// The next bench looks again.
func TestResolveHDRSample_OncePerBench(t *testing.T) {
	dir := t.TempDir()
	r := sampleTestRunner(t, dir, "ffmpeg")
	calls := 0
	r.hdrGenerate = func(string) error { calls++; return errors.New("x265 crashed") }

	path, fallback := r.resolveHDRSample(context.Background())
	if path != "" || !fallback {
		t.Fatalf("a failed generation falls back: got %q fallback=%v", path, fallback)
	}
	path, fallback = r.resolveHDRSample(context.Background())
	if path != "" || !fallback || calls != 1 {
		t.Fatalf("the second caller gets the first verdict without a retry: calls=%d got %q fallback=%v", calls, path, fallback)
	}

	r.forgetHDRSample()
	r.hdrGenerate = func(dst string) error { calls++; return os.WriteFile(dst, []byte("hdr"), 0o600) }
	path, fallback = r.resolveHDRSample(context.Background())
	if path != r.hdrSamplePath() || fallback || calls != 2 {
		t.Fatalf("a new bench generates again: calls=%d got %q fallback=%v", calls, path, fallback)
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
