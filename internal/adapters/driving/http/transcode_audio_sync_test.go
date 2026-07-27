package httpapi

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// The compensation must stay OFF unless something positively establishes that
// the video is packaged by the ffmpeg copy fallback. Shifting audio against an
// mkvgo-packaged video would CREATE the desync it exists to remove, so every
// unknown has to resolve to 0 - the pre-fix behaviour.
func TestVideoCompositionDelaySec_DefaultsToNoShift(t *testing.T) {
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/media/x.mkv"}}
	r := httptest.NewRequest("GET", "/hls/audio_0.m3u8", nil)

	cases := []struct {
		name string
		h    *hlsVODServe
		item *domainlibrary.MediaItem
	}{
		{
			name: "no probe wired",
			h:    &hlsVODServe{},
			item: item,
		},
		{
			name: "no on-demand packager wired: \"mkvgo did not take it\" would be an artefact, not an answer",
			h: &hlsVODServe{compositionDelayProbe: func(context.Context, string) float64 {
				t.Error("probe must not run without a packager to compare against")
				return 0.083
			}},
			item: item,
		},
		{
			name: "no media file",
			h: &hlsVODServe{compositionDelayProbe: func(context.Context, string) float64 {
				t.Error("probe must not run without a source path")
				return 0.083
			}},
			item: &domainlibrary.MediaItem{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.h.videoCompositionDelaySec(r, "m1", tc.item, nil); got != 0 {
				t.Errorf("videoCompositionDelaySec = %v, want 0 (no shift)", got)
			}
		})
	}
}

// sourceIdentity keys the plan-build memo. It must key on the file's identity,
// not just its path: a repaired or re-encoded source must re-prove whether it
// packages instead of inheriting the previous verdict.
func TestSourceIdentity_TracksFileIdentityNotJustPath(t *testing.T) {
	path := t.TempDir() + "/a.mkv"
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, ok := sourceIdentity(path)
	if !ok {
		t.Fatal("sourceIdentity failed on an existing file")
	}

	// Size differs, so the identity must differ even if the clock is coarse.
	time.Sleep(5 * time.Millisecond)
	if err := os.WriteFile(path, []byte("a longer body"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, ok := sourceIdentity(path)
	if !ok {
		t.Fatal("sourceIdentity failed after rewrite")
	}
	if first == second {
		t.Errorf("identity unchanged after the file changed (%q) - a repaired source would inherit a stale verdict", first)
	}

	if _, ok := sourceIdentity(path + ".missing"); ok {
		t.Error("a missing file must not yield an identity - it would poison the memo")
	}
}
