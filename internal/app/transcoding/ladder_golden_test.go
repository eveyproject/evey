package transcoding

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The ladder golden file pins, for a matrix of hosts, sources, clients and
// classes, everything the plan resolution decides about a session: the
// decision, the effective codec and route, the shaped ladder and the cache
// hash. It is the contract the ladder-engine refactor works under - the
// abstractions move, the bytes do not. Regenerate on a DELIBERATE change with
// `go test ./internal/app/transcoding -run TestLadderGolden -update` and read
// the diff before committing it.
var updateGolden = flag.Bool("update", false, "rewrite the golden files")

type goldenHost struct {
	name string
	pr   func() *PlanResolver
}

type goldenSource struct {
	name    string
	streams []*library.MediaStream
	kbps    int64
	bytes   int64
}

type goldenClient struct {
	name   string
	client transcoding.ClientProfile
	prefs  TranscodingPreferences
	screen int
	capKb  int
}

type goldenComplexity struct {
	class   transcoding.ContentClass
	metrics transcoding.ComplexityMetrics
}

func (g goldenComplexity) Get(context.Context, string) (transcoding.ComplexityEntry, bool, error) {
	return transcoding.ComplexityEntry{Class: g.class, Metrics: g.metrics}, g.class != "" || g.metrics.FramesSampled > 0, nil
}
func (goldenComplexity) Set(context.Context, string, transcoding.ContentClass, transcoding.ComplexityMetrics, time.Time) error {
	return nil
}
func (goldenComplexity) ListUnclassified(context.Context, int) ([]string, error) { return nil, nil }

func goldenVideo(codec string, w, h int, mutate func(*library.MediaStream)) *library.MediaStream {
	s := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: codec, Width: &w, Height: &h, Profile: "High", Level: new(41)}
	if codec == "hevc" {
		s.Profile = "Main"
		s.Level = new(153)
	}
	s.ColorTransfer = new("bt709")
	s.Keyframes = []int64{0, 3000, 6000}
	if mutate != nil {
		mutate(s)
	}
	return s
}

func goldenAudio(codec string, ch int, def bool) *library.MediaStream {
	return &library.MediaStream{StreamType: library.StreamTypeAudio, Codec: codec, Channels: &ch, IsDefault: def}
}

func goldenSources() []goldenSource {
	hdr := func(s *library.MediaStream) {
		s.ColorTransfer = new("smpte2084")
		s.Profile = "Main 10"
		s.BitDepth = new(10)
		s.ColorSpace = new("bt2020nc")
	}
	return []goldenSource{
		{"480p-h264-2m", []*library.MediaStream{goldenVideo("h264", 854, 480, nil), goldenAudio("aac", 2, true)}, 2000, 900e6},
		{"1080p-h264-8m", []*library.MediaStream{goldenVideo("h264", 1920, 1080, nil), goldenAudio("ac3", 6, true)}, 8000, 7e9},
		{"1080p-h264-25m-bluray", []*library.MediaStream{goldenVideo("h264", 1920, 1080, nil), goldenAudio("dts", 6, true), goldenAudio("ac3", 6, false)}, 25000, 22e9},
		{"2160p-hevc-sdr-15m", []*library.MediaStream{goldenVideo("hevc", 3840, 2160, nil), goldenAudio("eac3", 6, true)}, 15000, 13e9},
		{"2160p-hevc-hdr10-70m", []*library.MediaStream{goldenVideo("hevc", 3840, 2160, hdr), goldenAudio("truehd", 8, true), goldenAudio("ac3", 6, false)}, 70000, 70e9},
		{"2160p-hevc-hdr10-nokeyframes", []*library.MediaStream{goldenVideo("hevc", 3840, 2160, func(s *library.MediaStream) { hdr(s); s.Keyframes = nil }), goldenAudio("ac3", 6, true)}, 40000, 40e9},
		{"2160p-hevc-dovi7", []*library.MediaStream{goldenVideo("hevc", 3840, 2160, func(s *library.MediaStream) { hdr(s); s.DoViProfile = new(7) }), goldenAudio("ac3", 6, true)}, 50000, 50e9},
		{"1080p-h264-telecine", []*library.MediaStream{goldenVideo("h264", 1920, 1080, func(s *library.MediaStream) { s.IdetTelecine = new(true) }), goldenAudio("aac", 2, true)}, 6000, 5e9},
		{"1080p-h264-nobitrate", []*library.MediaStream{goldenVideo("h264", 1920, 1080, nil), goldenAudio("aac", 2, true)}, 0, 0},
		// Sources no GPU decoder takes: 10-bit H.264 (anime) and 4:2:2 HEVC (captures).
		{"1080p-hi10p", []*library.MediaStream{goldenVideo("h264", 1920, 1080, func(s *library.MediaStream) { s.Profile = "High 10"; s.BitDepth = new(10); s.PixelFormat = new("yuv420p10le") }), goldenAudio("aac", 2, true)}, 6000, 5e9},
		{"1080p-hevc-422", []*library.MediaStream{goldenVideo("hevc", 1920, 1080, func(s *library.MediaStream) { s.Profile = "Main 4:2:2 10"; s.BitDepth = new(10); s.PixelFormat = new("yuv422p10le") }), goldenAudio("aac", 2, true)}, 20000, 18e9},
	}
}

func goldenClients() []goldenClient {
	web := transcoding.ClientProfile{VideoCodecs: []string{"h264", "vp9"}, AudioCodecs: []string{"aac", "mp3", "opus", "flac"}, Containers: []string{"mp4", "webm"}, MaxAudioChannels: 6}
	webHEVC := web
	webHEVC.VideoCodecs = []string{"h264", "hevc"}
	hdrTV := transcoding.ClientProfile{VideoCodecs: []string{"h264", "hevc", "av1"}, AudioCodecs: []string{"aac", "ac3", "eac3"}, Containers: []string{"mp4", "mkv"}, SupportsHDR: true, MaxAudioChannels: 8}
	level41 := webHEVC
	level41.MaxH264Level = 41
	pick720 := webHEVC
	pick720.MaxHeight = 720
	prefHEVC := DefaultTranscodingPreferences()
	prefHEVC.PreferredVideoCodec = "hevc"
	cellular := DefaultTranscodingPreferences()
	cellular.NetworkType = "cellular"
	cellular.MaxBitrateCellularKbps = 3000
	profileCap := DefaultTranscodingPreferences()
	profileCap.MaxBitrateKbps = 5000
	return []goldenClient{
		{"web-h264", web, DefaultTranscodingPreferences(), 0, 0},
		{"web-hevc", webHEVC, DefaultTranscodingPreferences(), 0, 0},
		{"hdr-tv", hdrTV, DefaultTranscodingPreferences(), 0, 0},
		{"tv-level41", level41, DefaultTranscodingPreferences(), 0, 0},
		{"laptop-768", webHEVC, DefaultTranscodingPreferences(), 768, 0},
		{"pick-720", pick720, DefaultTranscodingPreferences(), 0, 0},
		{"remote-20m", webHEVC, DefaultTranscodingPreferences(), 0, 20000},
		{"profile-5m", webHEVC, profileCap, 0, 0},
		{"cellular-3m", webHEVC, cellular, 0, 0},
		{"pref-hevc", webHEVC, prefHEVC, 0, 0},
	}
}

func goldenResolver(cap *transcoding.EncodeCapability, hw bool, class transcoding.ContentClass, maxKbps int) *PlanResolver {
	return goldenResolverWith(cap, hw, goldenComplexity{class: class}, maxKbps)
}

func goldenResolverWith(cap *transcoding.EncodeCapability, hw bool, repo goldenComplexity, maxKbps int) *PlanResolver {
	pr := &PlanResolver{
		HWAccelEnabled:        func() bool { return hw },
		HWTonemapEnabled:      func() bool { return true },
		ComplexityRepo:        repo,
		UseFMP4:               true,
		AudioRenditionEnabled: true,
		MaxVideoBitrateKbps:   func() int { return maxKbps },
		CopyProducer:          &SoftwareProducer{copyPlanner: &CopyPlanner{}},
	}
	if cap != nil {
		c := *cap
		pr.Capability = func() transcoding.EncodeCapability { return c }
	}
	pr.Selector = NewProducerSelector(&SoftwareProducer{}, &SegmentOrchestrator{}, pr.HWRouteUsable)
	return pr
}

var nvencCapability = transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720, HWMaxHeight: 2160, HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc", "av1"}, HWDecodeCodecs: []string{"h264", "hevc"}, HW10BitTranscode: true, HWBridgeMaxHeight: 2160}

func goldenHosts(class transcoding.ContentClass) []goldenHost {
	soft := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480}
	vaapi := vaapiCapable()
	vaapi.HWHDR10Encode = true
	vaapi.HWFeatures = append(vaapi.HWFeatures, transcoding.FeatureTonemapOpenCL)
	vaapiH264 := vaapiCapable()
	vaapiH264.HWCodecs = []string{"h264"}
	vaapiH264.HWTonemap = false
	vaapiH264.HWBridgeMaxHeight = 1080
	nvenc := transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720, HWMaxHeight: 2160, HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc", "av1"}, HWDecodeCodecs: []string{"h264", "hevc"}, HW10BitTranscode: true, HWBridgeMaxHeight: 2160}
	// A row the v5 calibration wrote: decode classes and builder-proven routes
	// are authoritative. No bridge proof (the pre-fix NVENC chain failed), no
	// Main10 decode class: HDR tone-maps on the CPU, Main10 sources stay off
	// the zero-copy chain.
	nvencProbed := nvenc
	nvencProbed.ProbeVersion = transcoding.CapabilityProbeVersion
	nvencProbed.HWDecodeCodecs = []string{"h264", "hevc"}
	nvencProbed.HWRoutes = []string{transcoding.HWRouteSDR, transcoding.HWRouteHEVC, transcoding.HWRouteSource10Bit}
	return []goldenHost{
		{"fallback", func() *PlanResolver { return goldenResolver(nil, false, class, 0) }},
		{"software-1080", func() *PlanResolver { return goldenResolver(&soft, false, class, 0) }},
		{"vaapi-full", func() *PlanResolver { return goldenResolver(&vaapi, true, class, 0) }},
		{"vaapi-h264-bridge", func() *PlanResolver { return goldenResolver(&vaapiH264, true, class, 0) }},
		{"nvenc", func() *PlanResolver { return goldenResolver(&nvenc, true, class, 0) }},
		{"vaapi-full-cap40m", func() *PlanResolver { return goldenResolver(&vaapi, true, class, 40000) }},
		{"nvenc-probed-nobridge", func() *PlanResolver { return goldenResolver(&nvencProbed, true, class, 0) }},
	}
}

func goldenRow(pr *PlanResolver, src goldenSource, cl goldenClient) string {
	req := PlaybackRequest{
		MediaID: "m1", SourcePath: "/golden.mkv", Container: "matroska",
		Streams: src.streams, Client: cl.client, Prefs: cl.prefs,
		AudioTrack: 0, SubtitleBurnInTrack: -1,
		MaxBitrateKbps: cl.capKb, SourceBytes: src.bytes, SourceBitrateKbps: src.kbps, ScreenHeight: cl.screen,
	}
	plan, err := pr.Resolve(context.Background(), req)
	if err != nil {
		return "ERR " + err.Error()
	}
	d := plan.Decision
	var rungs []string
	for _, p := range plan.Ladder {
		copyTag := ""
		if p.Copy {
			copyTag = " copy"
		}
		rungs = append(rungs, fmt.Sprintf("%s %dx%d v%d a%d%s", p.Name, p.Width, p.Height, p.VideoBitrate, p.AudioBitrate, copyTag))
	}
	fp := plan.Fingerprint
	return fmt.Sprintf("%s/%s/%s | audio=%s codec=%s hw=%s class=%s tonemap=%t hwtm=%t pass=%t swdec=%t acopy=%t atrack=%d | %s | %s",
		d.PlayMethod, d.VideoDecision, strings.Join(d.TranscodeReasons, ","),
		fp.AudioCodec, plan.VideoCodec, plan.HWAccel, plan.Class, fp.HDRTonemap, fp.HWTonemap, fp.HDRPassthrough, fp.HWSoftwareDecode, fp.AudioCopy, fp.AudioTrack,
		strings.Join(rungs, " ; "), plan.FingerprintHash)
}

func TestLadderGolden(t *testing.T) {
	var out []string
	for _, host := range goldenHosts(transcoding.ClassFilmNormal) {
		for _, src := range goldenSources() {
			for _, cl := range goldenClients() {
				out = append(out, fmt.Sprintf("%s | %s | %s | %s", host.name, src.name, cl.name, goldenRow(host.pr(), src, cl)))
			}
		}
	}
	// Every class on three sources and two clients: the per-title table is the
	// first thing the ladder engine will replace.
	classes := []transcoding.ContentClass{transcoding.ClassAnimation, transcoding.ClassFilmLowMotion, transcoding.ClassFilmHighMotion, transcoding.ClassSport, ""}
	srcs := goldenSources()
	cls := goldenClients()
	for _, class := range classes {
		name := string(class)
		if name == "" {
			name = "unclassified"
		}
		for _, host := range goldenHosts(class)[1:3] {
			for _, src := range []goldenSource{srcs[1], srcs[2], srcs[4]} {
				for _, cl := range []goldenClient{cls[0], cls[2]} {
					out = append(out, fmt.Sprintf("class=%s %s | %s | %s | %s", name, host.name, src.name, cl.name, goldenRow(host.pr(), src, cl)))
				}
			}
		}
	}
	// Titles with persisted metrics: the cost model prices the CPU route (the
	// GPU keeps the table until the codec efficiency model exists). Three
	// motion levels, two hosts, three sources, three clients.
	motions := []float64{1.46, 4.05, 8.46} // a remux, Avatar, another title
	for _, ydif := range motions {
		repo := goldenComplexity{class: transcoding.ClassFilmNormal, metrics: transcoding.ComplexityMetrics{MotionMean: ydif, FramesSampled: 192, MetricVersion: transcoding.CanonicalMetricVersion}}
		hosts := []goldenHost{
			{"software-1080", func() *PlanResolver {
				return goldenResolverWith(&transcoding.EncodeCapability{SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 480}, false, repo, 0)
			}},
			{"vaapi-full", func() *PlanResolver {
				v := vaapiCapable()
				v.HWHDR10Encode = true
				return goldenResolverWith(&v, true, repo, 0)
			}},
			// NVENC is calibrated: its rungs are the cost price converted by the
			// measured H.264 / HEVC efficiency.
			{"nvenc", func() *PlanResolver { return goldenResolverWith(&nvencCapability, true, repo, 0) }},
		}
		for _, host := range hosts {
			for _, src := range []goldenSource{srcs[1], srcs[2], srcs[4]} {
				for _, cl := range []goldenClient{cls[0], cls[2], cls[6]} {
					out = append(out, fmt.Sprintf("ydif=%.2f %s | %s | %s | %s", ydif, host.name, src.name, cl.name, goldenRow(host.pr(), src, cl)))
				}
			}
		}
	}
	sort.Strings(out)
	got := strings.Join(out, "\n") + "\n"

	path := filepath.Join("testdata", "ladder_golden.txt")
	if *updateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		t.Logf("golden rewritten: %d rows", len(out))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file: run with -update once")
	if string(want) != got {
		wantLines := strings.Split(string(want), "\n")
		gotLines := strings.Split(got, "\n")
		var diff []string
		for i := range max(len(wantLines), len(gotLines)) {
			w, g := "", ""
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if w != g {
				diff = append(diff, "- "+w, "+ "+g)
			}
			if len(diff) > 40 {
				break
			}
		}
		t.Fatalf("ladder golden drifted (%d vs %d rows). First differences:\n%s", len(wantLines), len(gotLines), strings.Join(diff, "\n"))
	}
}
