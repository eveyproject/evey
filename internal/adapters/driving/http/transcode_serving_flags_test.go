package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	transcodingfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
)

// The packaging switches (transcoding.remux_copy, transcoding.hls_ondemand)
// are admin settings: a change must reach the NEXT playback without a
// restart, and must never move a playback already running. A running
// session's child URLs re-resolve the plan on every request; if they read the
// live setting, a toggle re-fingerprints them and the player gets 410 on
// every URL it holds. The master is where a playback starts, so the master is
// where the setting is read, and its decision rides the child URLs.

// servingFlagsCopyQuery is a client that decodes the source as is: the copy
// is possible, only the switches decide whether it is offered.
const servingFlagsCopyQuery = "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4,fmp4"

var servingFlagsVariantQueryRE = regexp.MustCompile(`stream_0\.m3u8\?([^\s"]*)`)

// newServingFlagsHandler builds the handler the way the server does
// (newHLSVODServe), on a config repository the test edits as the admin
// panel would.
func newServingFlagsHandler(t *testing.T, cfg shared.ServerConfigRepository, packager apptranscoding.OnDemandPackager, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) *hlsVODServe {
	t.Helper()
	cache, err := apptranscoding.NewSegmentCache(t.TempDir() + "/vod")
	require.NoError(t, err)
	orch := apptranscoding.NewSegmentOrchestrator(cache, &transcodingfakes.HLSProcess{}, &noopSourceResolver{},
		apptranscoding.OrchestratorOptions{WaitBudgetDur: 50 * time.Millisecond}, slog.Default())
	t.Cleanup(func() { orch.Close() })
	s := &HTTPServer{
		HLSVODOrch:       orch,
		HLSVODCache:      cache,
		ServerCfgRepo:    cfg,
		HLSTranscoder:    &transcodingfakes.HLSProcess{},
		OnDemandPackager: packager,
	}
	h := newHLSVODServe(s, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	require.NotNil(t, h)
	// No refresh delay here: the toggle must reach the very next master.
	for _, f := range []*liveServerFlag{h.remuxCopyFlag, h.packagerFlag} {
		if f != nil {
			f.refresh = 0
		}
	}
	// A fake encoder writes nothing: a segment request answers 503 after
	// this wait instead of the production budget.
	h.segmentWaitTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		if s.transcodeCloser != nil {
			s.transcodeCloser()
		}
	})
	return h
}

func servingFlagsRequest(file, rawQuery string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/"+file+"?"+rawQuery, nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", file)
	return withHTTPClaims(req, testClaims(false))
}

// doBoundedVODRequest is doVODRequestQ under a short deadline: a fake
// encoder never writes the segment, and what is asserted here is only whether
// the request was refused as stale (410), which happens before any wait.
func doBoundedVODRequest(t *testing.T, h *hlsVODServe, file, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := servingFlagsRequest(file, rawQuery)
	ctx, cancel := context.WithTimeout(req.Context(), 200*time.Millisecond)
	defer cancel()
	w := httptest.NewRecorder()
	h.serveHTTP(w, req.WithContext(ctx))
	return w
}

func copyFlagItem() *domainlibrary.MediaItem {
	dur, _ := shared.NewDurationMs(60_000)
	return &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
}

// copyMasterSession fetches a master and returns the query its first variant
// URI carries, with the plan that variant resolves now.
func copyMasterSession(t *testing.T, h *hlsVODServe, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) (string, *requestPlan) {
	t.Helper()
	w := doVODRequestQ(h, "m1", "master.m3u8", servingFlagsCopyQuery)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	m := servingFlagsVariantQueryRE.FindStringSubmatch(w.Body.String())
	require.NotNil(t, m, "the master must name its first variant:\n%s", w.Body.String())
	return m[1], planFor(t, h, servingFlagsRequest("stream_0.m3u8", m[1]), item, streams)
}

func setServingFlag(t *testing.T, cfg shared.ServerConfigRepository, key, value string) {
	t.Helper()
	require.NoError(t, cfg.Set(context.Background(), key, value))
}

func TestRemuxCopyFlag_AppliesToTheNextMasterAndNeverMovesARunningSession(t *testing.T) {
	for _, tc := range []struct {
		name            string
		before, after   string
		copyBefore      bool
		copyAfterToggle bool
	}{
		{"on then off", "on", "off", true, false},
		{"off then on", "off", "on", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EVEY_REMUX_COPY", "")
			cfg := sharedfakes.NewFakeServerConfigRepository()
			setServingFlag(t, cfg, "transcoding.remux_copy", tc.before)
			item, streams := copyFlagItem(), copyEligibleStreams()
			h := newServingFlagsHandler(t, cfg, nil, item, streams)

			query, started := copyMasterSession(t, h, item, streams)
			require.Equal(t, tc.copyBefore, started.IsCopy, "test rig: the master must follow the setting it starts under")
			fp := regexp.MustCompile(`(?:^|&)fp=([0-9a-f]+)`).FindStringSubmatch(query)
			require.NotNil(t, fp, "the variant URI must seal its fingerprint: %s", query)
			require.Equal(t, fp[1], started.FPHash)

			setServingFlag(t, cfg, "transcoding.remux_copy", tc.after)

			// The running session: same plan, same fingerprint, no 410.
			still := planFor(t, h, servingFlagsRequest("stream_0.m3u8", query), item, streams)
			assert.Equal(t, started.IsCopy, still.IsCopy, "a running session must keep the rung it started on")
			assert.Equal(t, started.FPHash, still.FPHash, "a running session must keep its fingerprint")
			for _, file := range []string{"stream_0.m3u8", "stream_0_init.mp4", "stream_0_000.m4s"} {
				w := doBoundedVODRequest(t, h, file, query)
				assert.NotEqual(t, http.StatusGone, w.Code, "%s of a running session refused after the toggle", file)
			}

			// The next playback follows the new setting, without a restart.
			_, next := copyMasterSession(t, h, item, streams)
			assert.Equal(t, tc.copyAfterToggle, next.IsCopy, "the next master must follow the new setting")
			assert.NotEqual(t, started.FPHash, next.FPHash)
		})
	}
}

// packagerMasterBody marks the fake packager's master, so a test reads which
// engine served a master off its bytes.
const packagerMasterBody = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nod_playlist.m3u8\n"

func newPackagerFlagHandler(t *testing.T, cfg shared.ServerConfigRepository) (*hlsVODServe, *domainlibrary.MediaItem, []*domainlibrary.MediaStream) {
	t.Helper()
	item := copyFlagItem()
	streams := copyEligibleAV()
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{resources: map[string][]byte{
		"master.m3u8":   []byte(packagerMasterBody),
		"playlist.m3u8": []byte("#EXTM3U\n#EXT-X-ENDLIST\n"),
	}}}
	return newServingFlagsHandler(t, cfg, pkg, item, streams), item, streams
}

// isPackagerMaster: the packager's master as served (its URIs rebaked), not
// an ffmpeg ladder.
func isPackagerMaster(body string) bool {
	return strings.Contains(body, "od_playlist.m3u8") && !strings.Contains(body, "stream_0.m3u8")
}

func TestOnDemandFlag_AppliesToTheNextMasterAndNeverMovesARunningSession(t *testing.T) {
	t.Run("on then off", func(t *testing.T) {
		t.Setenv("EVEY_HLS_ONDEMAND", "")
		cfg := sharedfakes.NewFakeServerConfigRepository()
		h, _, _ := newPackagerFlagHandler(t, cfg)

		w := doVODRequestQ(h, "m1", "master.m3u8", servingFlagsCopyQuery)
		require.Equal(t, http.StatusOK, w.Code)
		require.True(t, isPackagerMaster(w.Body.String()), "test rig: the packager must serve the master while on:\n%s", w.Body.String())

		setServingFlag(t, cfg, "transcoding.hls_ondemand", "off")

		// The running session keeps reading the packager's resources.
		w = doVODRequestQ(h, "m1", "od_playlist.m3u8", servingFlagsCopyQuery)
		assert.Equal(t, http.StatusOK, w.Code, "a running packager session must keep being served after the toggle")

		// The next playback goes to ffmpeg, without a restart.
		w = doVODRequestQ(h, "m1", "master.m3u8", servingFlagsCopyQuery)
		require.Equal(t, http.StatusOK, w.Code)
		assert.False(t, isPackagerMaster(w.Body.String()), "the next master must leave the packager once it is off")
	})

	t.Run("off then on", func(t *testing.T) {
		t.Setenv("EVEY_HLS_ONDEMAND", "")
		cfg := sharedfakes.NewFakeServerConfigRepository()
		setServingFlag(t, cfg, "transcoding.hls_ondemand", "off")
		h, item, streams := newPackagerFlagHandler(t, cfg)

		w := doVODRequestQ(h, "m1", "master.m3u8", servingFlagsCopyQuery)
		require.Equal(t, http.StatusOK, w.Code)
		require.False(t, isPackagerMaster(w.Body.String()), "test rig: an off packager must not serve the master")
		m := servingFlagsVariantQueryRE.FindStringSubmatch(w.Body.String())
		require.NotNil(t, m, "the ffmpeg master must name its first variant:\n%s", w.Body.String())
		started := planFor(t, h, servingFlagsRequest("stream_0.m3u8", m[1]), item, streams)
		require.Equal(t, apptranscoding.EngineFFmpeg, started.Plan.Delivery.Video,
			"the variant URI must resolve the plan its master served, not the packager's")

		setServingFlag(t, cfg, "transcoding.hls_ondemand", "on")

		// The running ffmpeg session resolves the plan it started on.
		still := planFor(t, h, servingFlagsRequest("stream_0.m3u8", m[1]), item, streams)
		assert.Equal(t, started.Plan.Delivery, still.Plan.Delivery, "a running ffmpeg session must not move to the packager")
		assert.Equal(t, started.FPHash, still.FPHash)
		for _, file := range []string{"stream_0.m3u8", "stream_0_init.mp4", "stream_0_000.m4s"} {
			rec := doBoundedVODRequest(t, h, file, m[1])
			assert.NotEqual(t, http.StatusGone, rec.Code, "%s of a running session refused after the toggle", file)
		}

		// Enabled at runtime after an off boot: the next playback is the
		// packager's, without a restart.
		w = doVODRequestQ(h, "m1", "master.m3u8", servingFlagsCopyQuery)
		require.Equal(t, http.StatusOK, w.Code)
		assert.True(t, isPackagerMaster(w.Body.String()), "the next master must reach the packager once it is on:\n%s", w.Body.String())
	})
}

// A switch is read from the store at most once per refresh: a burst of
// masters costs one read, and a toggle lands within the delay.
func TestLiveServerFlag_RereadsAfterTheRefreshDelay(t *testing.T) {
	t.Setenv("EVEY_TEST_LIVE_FLAG", "")
	cfg := sharedfakes.NewFakeServerConfigRepository()
	now := time.Unix(0, 0)
	f := newLiveServerFlag(cfg, "transcoding.test_live_flag", "EVEY_TEST_LIVE_FLAG", true)
	f.now = func() time.Time { return now }

	assert.True(t, f.enabled(), "unset keeps the default")
	setServingFlag(t, cfg, "transcoding.test_live_flag", "off")
	assert.True(t, f.enabled(), "within the refresh delay the last read stands")
	now = now.Add(servingFlagRefresh)
	assert.False(t, f.enabled(), "past the delay the toggle is read")

	var unwired *liveServerFlag
	assert.True(t, unwired.enabled(), "no flag: the wiring alone decides")
}

// The packager's own entry points (od_master.m3u8, od_manifest.mpd) are
// fetched bare by clients (the subtitle hook reads the packager's master for
// its renditions): with the switch off they must answer 404 so the client
// falls back, as before the switch was live. The resources a session already
// started on the packager keep being served.
func TestOnDemandFlag_OffRefusesThePackagerEntryPointsNotARunningSession(t *testing.T) {
	t.Setenv("EVEY_HLS_ONDEMAND", "")
	cfg := sharedfakes.NewFakeServerConfigRepository()
	h, _, _ := newPackagerFlagHandler(t, cfg)

	w := doVODRequestQ(h, "m1", "master.m3u8", servingFlagsCopyQuery)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, isPackagerMaster(w.Body.String()), "test rig: the packager must serve the master while on")

	setServingFlag(t, cfg, "transcoding.hls_ondemand", "off")

	for _, entry := range []string{"od_master.m3u8", "od_manifest.mpd"} {
		w = doVODRequestQ(h, "m1", entry, "")
		assert.Equal(t, http.StatusNotFound, w.Code, "%s must be refused while the packager is off", entry)
	}
	w = doVODRequestQ(h, "m1", "od_playlist.m3u8", servingFlagsCopyQuery)
	assert.Equal(t, http.StatusOK, w.Code, "a running packager session keeps its resources")
}

// serverCfgStub answers Get through fn, the rest of the port unused.
type serverCfgStub struct {
	shared.ServerConfigRepository
	fn func(ctx context.Context, key string) (*shared.ServerConfigEntry, error)
}

func (s serverCfgStub) Get(ctx context.Context, key string) (*shared.ServerConfigEntry, error) {
	return s.fn(ctx, key)
}

// A store that hangs must not hang the masters: one read is in flight at a
// time, its caller waits at most the read timeout, and every other master
// answers with the last known value.
func TestLiveServerFlag_HungStoreDoesNotHoldTheMasters(t *testing.T) {
	t.Setenv("EVEY_TEST_LIVE_FLAG", "")
	release := make(chan struct{})
	var calls atomic.Int32
	f := newLiveServerFlag(serverCfgStub{fn: func(context.Context, string) (*shared.ServerConfigEntry, error) {
		calls.Add(1)
		<-release
		return &shared.ServerConfigEntry{Value: "off"}, nil
	}}, "transcoding.test_live_flag", "EVEY_TEST_LIVE_FLAG", true)
	f.refresh, f.readTimeout = 0, 50*time.Millisecond
	defer close(release)

	start := time.Now()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { assert.True(t, f.enabled(), "nothing read yet: the default") })
	}
	wg.Wait()
	assert.Less(t, time.Since(start), time.Second, "a hung store held the masters past the read timeout")
	assert.Equal(t, int32(1), calls.Load(), "one read in flight at a time")
}

// A failed read keeps the last value read: a switch the admin turned off
// stays off through a transient store error instead of falling back to its
// default (on).
func TestLiveServerFlag_ReadErrorKeepsTheLastValue(t *testing.T) {
	t.Setenv("EVEY_TEST_LIVE_FLAG", "")
	var fail atomic.Bool
	f := newLiveServerFlag(serverCfgStub{fn: func(context.Context, string) (*shared.ServerConfigEntry, error) {
		if fail.Load() {
			return nil, errors.New("store unavailable")
		}
		return &shared.ServerConfigEntry{Value: "off"}, nil
	}}, "transcoding.test_live_flag", "EVEY_TEST_LIVE_FLAG", true)
	f.refresh = 0

	require.False(t, f.enabled())
	fail.Store(true)
	assert.False(t, f.enabled(), "a read error must keep the switch off, not reopen it")

	never := newLiveServerFlag(serverCfgStub{fn: func(context.Context, string) (*shared.ServerConfigEntry, error) {
		return nil, errors.New("store unavailable")
	}}, "transcoding.test_live_flag", "EVEY_TEST_LIVE_FLAG", true)
	assert.True(t, never.enabled(), "nothing ever read: the default")
}
