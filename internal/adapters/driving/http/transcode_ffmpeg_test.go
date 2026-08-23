package httpapi

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/pkg/hdr"
)

// Tests dédiés buildFFmpegArgs - couverture des branches HDR/tone-map
// (les autres branches sont déjà couvertes par coverage_boost2_test.go
// et transcode_burnin_test.go). Audit A12 = faux positif: le fichier
// transcode_ffmpeg.go contient uniquement buildFFmpegArgs (logique pure,
// pas un handler HTTP). Scénarios auth/404/403/ffmpeg-invoke inapplicables.

func hdrInfo10Bit() *hdr.HDRInfo {
	return &hdr.HDRInfo{IsHDR: true, Type: "hdr10", ColorSpace: "bt2020nc", BitDepth: 10}
}

func argsHas(args []string, needle string) bool {
	for _, a := range args {
		if a == needle {
			return true
		}
	}
	return false
}

func argsFind(args []string, needle string) (string, bool) {
	for _, a := range args {
		if strings.Contains(a, needle) {
			return a, true
		}
	}
	return "", false
}

func TestBuildFFmpegArgs_ToneMapNoScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: -1},
		transcodeAnalysis{needsToneMap: true, hdrInfo: hdrInfo10Bit()},
	)
	vf, ok := argsFind(args, "tonemap=hable")
	if !ok {
		t.Fatalf("expected tonemap filter in args, got: %v", args)
	}
	if strings.Contains(vf, "scale=-2:") {
		t.Errorf("unexpected scale prefix when videoScale=0: %s", vf)
	}
	if !argsHas(args, "libx264") {
		t.Error("expected libx264 when tone-mapping")
	}
}

func TestBuildFFmpegArgs_ToneMapWithScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: -1, videoScale: 1080},
		transcodeAnalysis{needsToneMap: true, hdrInfo: hdrInfo10Bit()},
	)
	vf, ok := argsFind(args, "scale=-2:1080")
	if !ok {
		t.Fatalf("expected scale=-2:1080 prefix in vf, got: %v", args)
	}
	if !strings.Contains(vf, "tonemap=hable") {
		t.Errorf("expected tonemap after scale, got: %s", vf)
	}
}

func TestBuildFFmpegArgs_ToneMapRemuxOnlySkipsToneMap(t *testing.T) {
	// remuxOnly=true => la branche tone-map est contournée
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: -1, remuxOnly: true},
		transcodeAnalysis{needsToneMap: true, hdrInfo: hdrInfo10Bit()},
	)
	if _, ok := argsFind(args, "tonemap="); ok {
		t.Error("expected no tonemap filter when remuxOnly=true")
	}
	// fallback: -c:v copy
	for i, a := range args {
		if a == "-c:v" && i+1 < len(args) && args[i+1] == "copy" {
			return
		}
	}
	t.Errorf("expected -c:v copy when remuxOnly, got: %v", args)
}

func TestBuildFFmpegArgs_BurnInToneMapNoScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: 0},
		transcodeAnalysis{
			burnInSubtitle: true,
			needsToneMap:   true,
			hdrInfo:        hdrInfo10Bit(),
		},
	)
	fc := ""
	for i, a := range args {
		if a == "-filter_complex" && i+1 < len(args) {
			fc = args[i+1]
			break
		}
	}
	if fc == "" {
		t.Fatalf("expected -filter_complex in args: %v", args)
	}
	if !strings.Contains(fc, "tonemap=hable") {
		t.Errorf("expected tonemap inside filter_complex, got: %s", fc)
	}
	if !strings.Contains(fc, "[vt]") || !strings.Contains(fc, "[vb]") {
		t.Errorf("expected [vt] and [vb] labels chaining tonemap→overlay, got: %s", fc)
	}
	if !strings.Contains(fc, "[0:s:0]overlay") {
		t.Errorf("expected overlay on subtitle stream 0, got: %s", fc)
	}
	if strings.Contains(fc, "scale=-2:") {
		t.Errorf("unexpected scale when videoScale=0: %s", fc)
	}
}

func TestBuildFFmpegArgs_BurnInToneMapWithScale(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: 1, videoScale: 720},
		transcodeAnalysis{
			burnInSubtitle: true,
			needsToneMap:   true,
			hdrInfo:        hdrInfo10Bit(),
		},
	)
	fc := ""
	for i, a := range args {
		if a == "-filter_complex" && i+1 < len(args) {
			fc = args[i+1]
			break
		}
	}
	if fc == "" {
		t.Fatalf("expected -filter_complex in args: %v", args)
	}
	if !strings.Contains(fc, "tonemap=hable") {
		t.Errorf("expected tonemap in filter, got: %s", fc)
	}
	if !strings.Contains(fc, "[0:s:1]overlay,scale=-2:720") {
		t.Errorf("expected overlay on sub 1 + scale=-2:720, got: %s", fc)
	}
}

func TestBuildFFmpegArgs_BurnInNoScaleNoToneMap(t *testing.T) {
	// default burn-in path (line 31): overlay sans tone-map ni scale
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: 2},
		transcodeAnalysis{burnInSubtitle: true},
	)
	fc := ""
	for i, a := range args {
		if a == "-filter_complex" && i+1 < len(args) {
			fc = args[i+1]
			break
		}
	}
	if fc == "" {
		t.Fatalf("expected -filter_complex: %v", args)
	}
	if strings.Contains(fc, "tonemap=") {
		t.Errorf("unexpected tonemap: %s", fc)
	}
	if strings.Contains(fc, "scale=") {
		t.Errorf("unexpected scale: %s", fc)
	}
	if !strings.HasSuffix(fc, "[0:s:2]overlay[vb]") {
		t.Errorf("expected plain overlay ending, got: %s", fc)
	}
}

func TestBuildFFmpegArgs_AudioTrackMapping(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: -1, audioTrack: 3},
		transcodeAnalysis{},
	)
	// -map 0:a:3 doit exister
	found := false
	for i, a := range args {
		if a == "-map" && i+1 < len(args) && args[i+1] == "0:a:3" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -map 0:a:3 in args, got: %v", args)
	}
}

func TestBuildFFmpegArgs_OutputContainerFlags(t *testing.T) {
	// Tous les chemins produisent fMP4 fragmenté vers stdout
	h := &transcodeServe{}
	cases := []struct {
		name string
		p    transcodeParams
		a    transcodeAnalysis
	}{
		{"copy", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{}},
		{"transcode", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsVideoTranscode: true}},
		{"tonemap", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsToneMap: true, hdrInfo: hdrInfo10Bit()}},
		{"burnin", transcodeParams{subtitleTrack: 0}, transcodeAnalysis{burnInSubtitle: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := h.buildFFmpegArgs("/v.mkv", tc.p, tc.a)
			if !argsHas(args, "-movflags") {
				t.Error("expected -movflags")
			}
			if !argsHas(args, "frag_keyframe+empty_moov+default_base_moof") {
				t.Error("expected fragmented mp4 movflags value")
			}
			if !argsHas(args, "-frag_duration") {
				t.Error("expected -frag_duration")
			}
			if args[len(args)-1] != "-" {
				t.Errorf("expected stdout output (-), got trailing: %s", args[len(args)-1])
			}
			if args[len(args)-2] != "mp4" || args[len(args)-3] != "-f" {
				t.Errorf("expected -f mp4 before stdout, got: %v", args[len(args)-3:])
			}
		})
	}
}

func TestBuildFFmpegArgs_InputOrdering(t *testing.T) {
	// -ss (si présent) DOIT précéder -i (seek rapide input)
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/v.mkv",
		transcodeParams{subtitleTrack: -1, startTimeSec: 12.25},
		transcodeAnalysis{},
	)
	ssIdx, iIdx := -1, -1
	for i, a := range args {
		if a == "-ss" && ssIdx == -1 {
			ssIdx = i
		}
		if a == "-i" && iIdx == -1 {
			iIdx = i
		}
	}
	if ssIdx == -1 || iIdx == -1 {
		t.Fatalf("expected both -ss and -i, got: %v", args)
	}
	if ssIdx >= iIdx {
		t.Errorf("-ss (%d) must precede -i (%d)", ssIdx, iIdx)
	}
	// valeur formatée 3 décimales
	if args[ssIdx+1] != "12.250" {
		t.Errorf("expected -ss 12.250, got %s", args[ssIdx+1])
	}
}

// The legacy on-the-fly transcode folds to stereo unconditionally; without the
// matrix headroom swr sums the bed above full scale and the client clips it.
// See domain/transcoding/audio_downmix.go for the measurements.
func TestBuildFFmpegArgs_StereoFoldCarriesMatrixHeadroom(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: -1},
		transcodeAnalysis{needsAudioTranscode: true},
	)
	if !argsHas(args, "-rematrix_maxval") {
		t.Fatalf("stereo fold shipped without matrix headroom: %v", args)
	}

	// A stream copy has no resampler to configure.
	copied := h.buildFFmpegArgs("/video.mkv",
		transcodeParams{subtitleTrack: -1},
		transcodeAnalysis{},
	)
	if argsHas(copied, "-rematrix_maxval") {
		t.Errorf("audio copy must not carry a resampler option: %v", copied)
	}
}
