package demo_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/demo"
)

// A Build entry: every input is downloaded, ffmpeg gets them in order with
// the chapter marks and the output format named, the result lands under its
// final name and the inputs are gone; a failed mux leaves nothing behind.
func TestDownloadManifest_BuildsAMuxFromItsInputs(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = rw.Write([]byte("bytes of " + r.URL.Path))
	}))
	defer srv.Close()

	var got []string
	var chapters string
	h.boot.WithHTTPClient(srv.Client()).WithAssembler(func(_ context.Context, args []string) error {
		got = args
		for i, a := range args {
			if a == "-i" && strings.HasSuffix(args[i+1], "chapters.txt") {
				b, _ := os.ReadFile(args[i+1])
				chapters = string(b)
			}
		}
		return os.WriteFile(args[len(args)-1], []byte("muxed"), 0o644)
	})
	manifest := []demo.MediaFile{{Path: "movies/Film (2012).mkv", Build: &demo.Build{
		Inputs:      []demo.BuildInput{{URL: srv.URL + "/video.mov", Ext: ".mov"}, {URL: srv.URL + "/fr.srt", Ext: ".srt"}},
		Args:        []string{"-map", "0:v:0", "-c", "copy"},
		Chapters:    []demo.Chapter{{StartSec: 0, Title: "One"}, {StartSec: 60, Title: "Two=2"}},
		DurationSec: 90,
	}}}
	libs := map[string]shared.LibraryID{"movies": "lib-m"}
	if err := h.boot.DownloadManifest(ctx, manifest, libs); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(h.dir, "movies", "Film (2012).mkv")); err != nil || string(b) != "muxed" {
		t.Fatalf("built file: %v %q", err, b)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "in00.mov") || !strings.Contains(joined, "in01.srt") || strings.Index(joined, "in00.mov") > strings.Index(joined, "in01.srt") {
		t.Errorf("inputs not passed in order: %s", joined)
	}
	if i := slices.Index(got, "-map_chapters"); i < 0 || got[i+1] != "2" {
		t.Errorf("chapters input not mapped: %s", joined)
	}
	if !strings.Contains(chapters, "START=60000") || !strings.Contains(chapters, "END=90000") || !strings.Contains(chapters, `title=Two\=2`) {
		t.Errorf("chapter metadata: %q", chapters)
	}
	if !strings.Contains(joined, "-f matroska") {
		t.Errorf("output format not named: %s", joined)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "movies", "Film (2012).mkv.build")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the downloaded inputs must be removed after the mux")
	}

	// A mux that fails leaves no media file, and is retried at the next boot.
	h.boot.WithAssembler(func(context.Context, []string) error { return errors.New("ffmpeg died") })
	manifest[0].Path = "movies/Other (2012).mkv"
	if err := h.boot.DownloadManifest(ctx, manifest, libs); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(h.dir, "movies"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "Other") {
			t.Errorf("a failed build left %s", e.Name())
		}
	}
}

// A title replaced by a richer file is deleted at boot and its library
// rescanned, so it does not show twice.
func TestDownloadManifest_RemovesRetiredMedia(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	old := filepath.Join(h.dir, filepath.FromSlash(demo.RetiredMedia[0]))
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.boot.DownloadManifest(ctx, nil, map[string]shared.LibraryID{"movies": "lib-m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Error("the retired file is still there")
	}
}
