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
