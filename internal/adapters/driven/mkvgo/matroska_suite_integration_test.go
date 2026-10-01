package mkvgo

// Matroska's own conformance files (IETF CELLAR, github.com/ietf-wg-cellar/
// matroska-test-files; Elephants Dream, CC-BY) that are damaged on purpose, so
// they never go in the public demo: the on-demand packager must still serve
// every segment of them, and keep a hole in the audio a hole.
//
//	test7: junk elements at both ends of Clusters and an invalid element at
//	       451417 - skipped, not fatal.
//	test8: audio frames missing between 6.019 s and 6.360 s - the gap must stay
//	       where it is, not be closed (that would shift the sound by 0.34 s for
//	       the rest of the film).
//
// OPT-IN, needs the network the first time (files are cached):
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run MatroskaSuite -v
//
// or `make transcode-itest`. EVEY_ITEST_CACHE moves the cache.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

const matroskaSuiteURL = "https://raw.githubusercontent.com/ietf-wg-cellar/matroska-test-files/master/test_files/"

// suiteFile returns the cached copy of a conformance file, downloading it the
// first time. Offline with nothing cached, the test is skipped, said so.
func suiteFile(t *testing.T, name string) string {
	t.Helper()
	dir := os.Getenv("EVEY_ITEST_CACHE")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "evey-itest", "matroska-test-files")
	}
	p := filepath.Join(dir, name)
	if st, err := os.Stat(p); err == nil && st.Size() > 0 {
		return p
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, matroskaSuiteURL+name, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("cannot download %s (offline?): %v", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("cannot download %s: HTTP %d", name, resp.StatusCode)
	}
	tmp := p + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, p); err != nil {
		t.Fatal(err)
	}
	return p
}

// servePlaylist fetches every resource a playlist names (its init and all its
// segments) through the plan and returns them concatenated.
func servePlaylist(t *testing.T, ctx context.Context, plan apptranscoding.OnDemandPlan, playlist string) []byte {
	t.Helper()
	pl, _, err := plan.Resource(ctx, playlist)
	if err != nil {
		t.Fatalf("%s: %v", playlist, err)
	}
	var out bytes.Buffer
	n := 0
	for l := range strings.SplitSeq(string(pl), "\n") {
		name := ""
		switch {
		case strings.HasPrefix(l, "#EXT-X-MAP:URI="):
			name = strings.Trim(strings.TrimPrefix(l, "#EXT-X-MAP:URI="), `"`)
		case l != "" && !strings.HasPrefix(l, "#"):
			name = l
		}
		if name == "" {
			continue
		}
		name = strings.TrimPrefix(strings.SplitN(name, "?", 2)[0], "od_")
		b, _, err := plan.Resource(ctx, name)
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: resource %s not served: %v", playlist, name, err)
		}
		out.Write(b)
		n++
	}
	if n < 3 {
		t.Fatalf("%s names %d resources, want an init and segments", playlist, n)
	}
	return out.Bytes()
}

// packetTimes lists the presentation times ffprobe reads from an fMP4 stream.
func packetTimes(t *testing.T, ctx context.Context, fmp4 []byte) []float64 {
	t.Helper()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "packet=pts_time", "-of", "csv=p=0", "-")
	cmd.Stdin = bytes.NewReader(fmp4)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var ts []float64
	for l := range strings.FieldsSeq(string(out)) {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(l, ","), 64); err == nil {
			ts = append(ts, v)
		}
	}
	return ts
}

func TestMatroskaSuiteIntegration_DamagedFilesStillPackage(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	for _, name := range []string{"test7.mkv", "test8.mkv"} {
		t.Run(name, func(t *testing.T) {
			src := suiteFile(t, name)
			pkg := NewOnDemandPackager()
			defer pkg.Close()
			plan, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"})
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			video := servePlaylist(t, ctx, plan, "playlist.m3u8")
			audio := servePlaylist(t, ctx, plan, "audio1.m3u8")

			vt, at := packetTimes(t, ctx, video), packetTimes(t, ctx, audio)
			if len(vt) == 0 || len(at) == 0 {
				t.Fatalf("no packets: video %d, audio %d", len(vt), len(at))
			}
			vEnd, aEnd := vt[len(vt)-1], at[len(at)-1]
			if d := vEnd - aEnd; d > 1 || d < -1 {
				t.Errorf("audio ends at %.3f s, video at %.3f s: the tracks drifted apart", aEnd, vEnd)
			}
			if name != "test8.mkv" {
				return
			}
			// The hole between 6.019 s and 6.360 s is still there, in place.
			gap := 0.0
			for i := 1; i < len(at); i++ {
				if at[i-1] >= 5.9 && at[i-1] <= 6.1 {
					gap = max(gap, at[i]-at[i-1])
				}
			}
			if gap < 0.25 {
				t.Errorf("largest audio step near 6 s is %.3f s: the 0.34 s gap was closed, the sound now runs early", gap)
			}
			fmt.Printf("test8: audio gap kept (%.3f s), audio ends %.3f s, video %.3f s\n", gap, aEnd, vEnd)
		})
	}
}
