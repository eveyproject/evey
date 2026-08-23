package transcoding

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// fakePlayabilityAnalyzer records every call and returns a queued report/err
// (or panics, when panicOn is set) for exercising the shadow comparer without
// mkvgo.
type fakePlayabilityAnalyzer struct {
	mu      sync.Mutex
	calls   int
	report  *PlayabilityReport
	err     error
	panicOn bool
}

func (f *fakePlayabilityAnalyzer) Playability(_ context.Context, _ string, _ PlayabilityTarget) (*PlayabilityReport, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.panicOn {
		panic("boom: simulated analyzer panic")
	}
	return f.report, f.err
}

func (f *fakePlayabilityAnalyzer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func copyDecision(playMethod, videoDecision string) *domaintranscoding.PlaybackDecision {
	return &domaintranscoding.PlaybackDecision{PlayMethod: playMethod, VideoDecision: videoDecision, AudioDecision: "copy"}
}

func enableShadow(t *testing.T, repo shared.ServerConfigRepository) {
	t.Helper()
	require.NoError(t, repo.Set(context.Background(), PlayabilityShadowConfigKey, "on"))
}

func TestPlayabilityShadow_GateOffMeansZeroAnalyzerCalls(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "direct-play",
		Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "direct-play"}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository() // key absent -> off by default
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()

	assert.Equal(t, 0, analyzer.callCount(), "gate off must not reach the analyzer")
}

// countingConfigRepo counts Get calls so a test can assert the gate is not
// re-read once per observation.
type countingConfigRepo struct {
	shared.ServerConfigRepository
	mu   sync.Mutex
	gets int
}

func (c *countingConfigRepo) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	c.mu.Lock()
	c.gets++
	c.mu.Unlock()
	return c.ServerConfigRepository.Get(ctx, key)
}

func (c *countingConfigRepo) getCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gets
}

// Observe runs once per transcode decision, which is once per HLS segment. With
// 3s segments that is ~1200 calls per hour per viewer. The gate check used to
// live inside the spawned goroutine, so a DISABLED feature still cost one
// goroutine and one uncached SELECT per segment. The gate must be consulted -
// and cached - before anything is spawned.
func TestPlayabilityShadow_GateOffDoesNotSpawnOrQueryPerObservation(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{}
	repo := &countingConfigRepo{ServerConfigRepository: sharedfakes.NewFakeServerConfigRepository()}
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	const observations = 200
	for range observations {
		shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	}
	shadow.Wait()

	assert.Equal(t, 0, analyzer.callCount(), "gate off must not reach the analyzer")
	assert.LessOrEqual(t, repo.getCount(), 2,
		"gate off must not re-read server config once per observation (got %d reads for %d observations)",
		repo.getCount(), observations)
}

// Caching the gate must not make it permanently off: once an operator turns the
// shadow on, the next observation past the TTL has to pick it up.
func TestPlayabilityShadow_GateTurnedOnIsPickedUpAfterTTL(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "direct-play",
		Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "direct-play"}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()
	require.Equal(t, 0, analyzer.callCount(), "precondition: gate starts off")

	enableShadow(t, repo)
	shadow.expireGateCache()

	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()
	assert.Equal(t, 1, analyzer.callCount(), "gate turned on must be observed once the cache expires")
}

func TestPlayabilityShadow_ConvergentDoesNotError(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "direct-play",
		Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "direct-play"}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()

	assert.Equal(t, 1, analyzer.callCount())
}

func TestPlayabilityShadow_DivergentIsThrottledPerMediaAndTarget(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "transcode",
		Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "transcode", Reasons: []string{"video codec unsupported"}}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	decision := copyDecision("direct_play", "copy") // Evey says direct-play, mkvgo says transcode: divergent
	client := domaintranscoding.ClientProfile{VideoCodecs: []string{"h264"}}

	shadow.Observe(context.Background(), "/media/movie.mkv", decision, client)
	shadow.Wait()
	shadow.Observe(context.Background(), "/media/movie.mkv", decision, client)
	shadow.Wait()

	// The analyzer itself is called every time (it's the log that throttles) -
	// assert the throttle map recorded exactly one entry for this (path,target).
	key := "/media/movie.mkv::" + targetFingerprint(mapClientProfileToTarget(client))
	shadow.mu.Lock()
	_, seen := shadow.seen[key]
	entries := len(shadow.seen)
	shadow.mu.Unlock()
	assert.True(t, seen, "divergence key should be recorded after the first log")
	assert.Equal(t, 1, entries, "second call within the TTL must not add a new throttle entry")
	assert.Equal(t, 2, analyzer.callCount())
}

func TestPlayabilityShadow_PanicInGoroutineDoesNotPropagate(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{panicOn: true}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	assert.NotPanics(t, func() {
		shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
		shadow.Wait()
	})
	assert.Equal(t, 1, analyzer.callCount())
}

func TestPlayabilityShadow_NilSafe(t *testing.T) {
	var shadow *PlayabilityShadow
	assert.NotPanics(t, func() {
		shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
		shadow.Wait()
	})
}

func TestPlayabilityShadow_EmptyPathOrNilDecisionNoop(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{OverallVerdict: "direct-play"}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Observe(context.Background(), "/media/movie.mkv", nil, domaintranscoding.ClientProfile{})
	shadow.Wait()

	assert.Equal(t, 0, analyzer.callCount())
}

func TestPlayabilityShadow_AnalyzerErrorIsSwallowed(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{err: errors.New("read failed")}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	assert.NotPanics(t, func() {
		shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
		shadow.Wait()
	})
	assert.Equal(t, 1, analyzer.callCount())
}

func TestPlayabilityShadow_NoVideoTrackInReportNoop(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "direct-play",
		Tracks:         []PlayabilityTrackVerdict{{Type: "audio", Verdict: "direct-play"}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()

	shadow.mu.Lock()
	entries := len(shadow.seen)
	shadow.mu.Unlock()
	assert.Equal(t, 0, entries, "no video track means nothing to compare, no throttle entry")
}

func TestEveyVideoVerdict(t *testing.T) {
	cases := []struct {
		name string
		d    *domaintranscoding.PlaybackDecision
		want string
	}{
		{"transcode wins regardless of play method", &domaintranscoding.PlaybackDecision{PlayMethod: "transcode", VideoDecision: "transcode"}, "transcode"},
		{"direct_stream with copy video is a remux", &domaintranscoding.PlaybackDecision{PlayMethod: "direct_stream", VideoDecision: "copy"}, "remux"},
		{"direct_play with copy video is direct-play", &domaintranscoding.PlaybackDecision{PlayMethod: "direct_play", VideoDecision: "copy"}, "direct-play"},
		// The HLS pipeline with a copied video stream repackages the container -
		// that IS a remux in ops.Playability's vocabulary. Folding it to
		// "direct-play" made every copy-into-HLS session log a spurious
		// divergence against mkvgo's (correct) "remux" (seen live 2026-07-26 on
		// an MKV whose container the client could not read).
		{"transcode method with copy video is a remux", &domaintranscoding.PlaybackDecision{PlayMethod: "transcode", VideoDecision: "copy"}, "remux"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, eveyVideoVerdict(c.d))
		})
	}
}

func TestMakePlaybackDecisionWithPrefs_SourcePathWiresShadowHook(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "direct-play",
		Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "direct-play"}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	SetPlayabilityShadow(shadow)
	t.Cleanup(func() { SetPlayabilityShadow(nil) })

	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", IsDefault: true},
		{StreamType: "audio", Codec: "aac", IsDefault: true},
	}
	MakePlaybackDecisionWithPrefs(context.Background(), streams, "mp4", domaintranscoding.DefaultWebProfile, 0, DefaultTranscodingPreferences(), "/media/movie.mkv")
	shadow.Wait()

	assert.Equal(t, 1, analyzer.callCount(), "a supplied sourcePath must reach the shadow hook")
}

func TestMakePlaybackDecisionWithPrefs_NoSourcePathSkipsShadowHook(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{OverallVerdict: "direct-play"}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	SetPlayabilityShadow(shadow)
	t.Cleanup(func() { SetPlayabilityShadow(nil) })

	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", IsDefault: true},
	}
	MakePlaybackDecisionWithPrefs(context.Background(), streams, "mp4", domaintranscoding.DefaultWebProfile, 0, DefaultTranscodingPreferences())
	shadow.Wait()

	assert.Equal(t, 0, analyzer.callCount(), "no sourcePath means the shadow hook must not fire")
}

func TestMakePlaybackDecisionWithPrefs_NilShadowIsInert(t *testing.T) {
	SetPlayabilityShadow(nil)
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", IsDefault: true},
	}
	assert.NotPanics(t, func() {
		MakePlaybackDecisionWithPrefs(context.Background(), streams, "mp4", domaintranscoding.DefaultWebProfile, 0, DefaultTranscodingPreferences(), "/media/movie.mkv")
	})
}

func TestMapClientProfileToTarget(t *testing.T) {
	client := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "hevc"},
		AudioCodecs: []string{"aac"},
		Containers:  []string{"mp4"},
		MaxWidth:    1920,
		MaxHeight:   1080,
		SupportsHDR: true,
	}
	target := mapClientProfileToTarget(client)
	assert.Equal(t, client.VideoCodecs, target.VideoCodecs)
	assert.Equal(t, client.AudioCodecs, target.AudioCodecs)
	assert.Equal(t, client.Containers, target.Containers)
	assert.Equal(t, client.MaxWidth, target.MaxWidth)
	assert.Equal(t, client.MaxHeight, target.MaxHeight)
	assert.True(t, target.HDR)
}

// The two artifact classes below are the ones the 2026-07-29 production
// investigation proved benign (34 WARNs read one by one, zero genuine
// disagreements). They must not reach WARN - asserted through the throttle map,
// which only the WARN path writes to, so "no entry" means "never warned".

func TestPlayabilityShadow_UnmappedCodecSignalIsNotWarned(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "transcode",
		Tracks: []PlayabilityTrackVerdict{{
			Type:    "video",
			Verdict: "transcode",
			// mkvgo's ops.Target got a zero HEVCMain10 because ClientProfile
			// carries no such signal, so it demands a transcode for a 10-bit
			// title Evey direct-plays correctly.
			Reasons: []string{"HEVC Main10 (10-bit) is unsupported on target"},
		}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "/media/4k-10bit.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()

	assert.Equal(t, 1, analyzer.callCount(), "the comparison still runs")
	shadow.mu.Lock()
	entries := len(shadow.seen)
	shadow.mu.Unlock()
	assert.Zero(t, entries, "a known mapping artifact must not be logged as a divergence")
}

func TestPlayabilityShadow_GlobalVsTrackAsymmetryIsNotWarned(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "direct-play",
		Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "direct-play"}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	// Evey folds HLS-with-copied-video to "remux" while mkvgo judges the video
	// track alone and says direct-play. Evey is the stricter of the two, which
	// can never cause a playback failure.
	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("transcode", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()

	shadow.mu.Lock()
	entries := len(shadow.seen)
	shadow.mu.Unlock()
	assert.Zero(t, entries, "Evey being stricter than mkvgo is not a divergence worth a WARN")
}

func TestPlayabilityShadow_RealDivergenceStillWarns(t *testing.T) {
	analyzer := &fakePlayabilityAnalyzer{report: &PlayabilityReport{
		OverallVerdict: "transcode",
		Tracks: []PlayabilityTrackVerdict{{
			Type:    "video",
			Verdict: "transcode",
			// A width ceiling IS mapped, so this one is a real disagreement: if
			// the filter ever swallowed it, a genuine bug would go unseen.
			Reasons: []string{"width 3840 exceeds target max 1920"},
		}},
	}}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	shadow := NewPlayabilityShadow(analyzer, repo, testLogger())

	shadow.Observe(context.Background(), "/media/movie.mkv", copyDecision("direct_play", "copy"), domaintranscoding.ClientProfile{})
	shadow.Wait()

	shadow.mu.Lock()
	entries := len(shadow.seen)
	shadow.mu.Unlock()
	assert.Equal(t, 1, entries, "a real disagreement must still reach WARN")
}

func TestKnownMappingArtifact(t *testing.T) {
	copied := &domaintranscoding.PlaybackDecision{VideoDecision: "copy"}
	transcoded := &domaintranscoding.PlaybackDecision{VideoDecision: "transcode"}

	tests := []struct {
		name         string
		evey, mkvgo  string
		reasons      []string
		decision     *domaintranscoding.PlaybackDecision
		wantArtifact string
	}{
		{"main10 only", "direct-play", "transcode",
			[]string{"HEVC Main10 (10-bit) is unsupported on target"}, copied, "unmapped-codec-signal"},
		{"dovi only", "direct-play", "transcode",
			[]string{"Dolby Vision is unsupported on target"}, copied, "unmapped-codec-signal"},
		{"vp9 profile2 only", "direct-play", "transcode",
			[]string{"VP9 Profile 2 (10-bit) is unsupported on target"}, copied, "unmapped-codec-signal"},
		{"several unmapped signals together", "direct-play", "transcode",
			[]string{"Dolby Vision is unsupported on target", "HEVC Main10 (10-bit) is unsupported on target"}, copied, "unmapped-codec-signal"},
		// One real reason alongside an artifact one keeps the whole divergence
		// visible: the real reason is the thing worth reading.
		{"unmapped signal mixed with a real reason", "direct-play", "transcode",
			[]string{"HEVC Main10 (10-bit) is unsupported on target", "height 2160 exceeds target max 1080"}, copied, ""},
		// HDR IS mapped from ClientProfile.SupportsHDR, so this is a real signal.
		{"hdr10 is a mapped signal", "direct-play", "transcode",
			[]string{"HDR10 static metadata is unsupported on target"}, copied, ""},
		{"transcode with no reason at all", "direct-play", "transcode", nil, copied, ""},
		{"remux vs direct-play with video copied", "remux", "direct-play", nil, copied, "global-vs-track-asymmetry"},
		// Evey transcoding the video while mkvgo direct-plays it is the opposite
		// direction: Evey is doing MORE work than needed, which is a real finding.
		{"remux vs direct-play with video transcoded", "remux", "direct-play", nil, transcoded, ""},
		{"direct-play vs remux is a real divergence", "direct-play", "remux", nil, copied, ""},
		{"nil decision never classifies as class 2", "remux", "direct-play", nil, nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantArtifact, knownMappingArtifact(tt.evey, tt.mkvgo, tt.reasons, newDecisionSnapshot(tt.decision)))
		})
	}
}

// blockingAnalyzer holds the shadow's goroutine inside Playability until the
// test releases it, so the test can mutate the decision in the window the real
// caller mutates it: PlanResolver.Decide applies the vd= pin to the SAME
// *PlaybackDecision pointer immediately after MakePlaybackDecisionWithPrefs
// returns, while the head-only mkvgo read is still in flight.
type blockingAnalyzer struct {
	entered chan struct{}
	release chan struct{}
	report  *PlayabilityReport
}

func (b *blockingAnalyzer) Playability(_ context.Context, _ string, _ PlayabilityTarget) (*PlayabilityReport, error) {
	close(b.entered)
	<-b.release
	return b.report, nil
}

// countingHandler counts WARN records, which is what an operator actually sees.
type countingHandler struct {
	mu    sync.Mutex
	warns int
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.mu.Lock()
		h.warns++
		h.mu.Unlock()
	}
	return nil
}
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }
func (h *countingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.warns
}

// Incident cast du 2026-08-23 : "playability shadow: divergent verdict" a
// ete emis a 19:27:49.970 sur un film servi en COPIE PURE - et ce faux WARN a
// envoye toute l'enquete dans le mur pendant des heures. Mecanisme reel :
// Observe confie l'ADRESSE de la decision a une goroutine detachee, qui ne lit
// VideoDecision/PlayMethod qu'APRES le retour du lecteur d'entete mkvgo (des
// millisecondes plus tard). Entre-temps, PlanResolver.Decide a applique le pin
// vd=transcode de la sonde du barreau de secours SUR CETTE MEME ADRESSE. L'ombre
// compare donc un verdict IMPOSE a celui de mkvgo. C'est aussi une course de
// donnees pure et simple (-race la voit).
// Le verdict doit etre pris en photo au moment de l'appel, pas relu plus tard.
func TestPlayabilityShadow_SnapshotsTheVerdictBeforeTheCallerPinsIt(t *testing.T) {
	analyzer := &blockingAnalyzer{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		report: &PlayabilityReport{
			OverallVerdict: "direct-play",
			Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "direct-play"}},
		},
	}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	h := &countingHandler{}
	shadow := NewPlayabilityShadow(analyzer, repo, slog.New(h))

	// Le plan reel : copie pure, direct-play - convergent avec mkvgo.
	decision := copyDecision("direct_play", "copy")
	shadow.Observe(context.Background(), "/media/films/Some Movie - 1080P.mkv", decision, domaintranscoding.ClientProfile{})

	<-analyzer.entered
	// La sonde du barreau de secours : vd=transcode force, applique par
	// PlanResolver.Decide sur l'adresse que l'ombre detient encore.
	decision.VideoDecision = "transcode"
	decision.TranscodeReasons = append(decision.TranscodeReasons, "decision_pinned")
	close(analyzer.release)
	shadow.Wait()

	assert.Zero(t, h.count(), "aucun WARN : la decision REELLE etait convergente, le pin est venu apres")
}

// L'inverse doit rester vrai : une vraie divergence, presente des l'appel,
// continue d'etre signalee. Un correctif qui rend l'ombre muette ne corrige
// rien, il aveugle.
func TestPlayabilityShadow_RealDivergencePresentAtCallTimeStillWarns(t *testing.T) {
	analyzer := &blockingAnalyzer{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		report: &PlayabilityReport{
			OverallVerdict: "transcode",
			Tracks:         []PlayabilityTrackVerdict{{Type: "video", Verdict: "transcode", Reasons: []string{"video codec unsupported on target"}}},
		},
	}
	repo := sharedfakes.NewFakeServerConfigRepository()
	enableShadow(t, repo)
	h := &countingHandler{}
	shadow := NewPlayabilityShadow(analyzer, repo, slog.New(h))

	decision := copyDecision("direct_play", "copy") // evey direct-play vs mkvgo transcode
	shadow.Observe(context.Background(), "/media/films/Some Movie - 1080P.mkv", decision, domaintranscoding.ClientProfile{})
	<-analyzer.entered
	close(analyzer.release)
	shadow.Wait()

	assert.Equal(t, 1, h.count(), "une divergence reelle doit rester visible")
}
