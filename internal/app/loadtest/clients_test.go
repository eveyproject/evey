package loadtest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseClientMix(t *testing.T) {
	mix, err := parseClientMix("tv=40, mobile=40,web-chrome=20,web-safari=0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(mix) != 3 || mix[0] != (clientShare{"tv", 40}) || mix[2] != (clientShare{"web-chrome", 20}) {
		t.Fatalf("mix = %v, want tv/mobile/web-chrome and the zero weight dropped", mix)
	}
	for _, bad := range []string{"", "tv", "tv=x", "tv=-1", "toaster=10", "tv=0"} {
		if _, err := parseClientMix(bad); err == nil {
			t.Errorf("parseClientMix(%q) accepted a mix it cannot play", bad)
		}
	}
}

// The history mix maps the recorded device types onto their profiles and
// ignores what no profile plays; a history thinner than minHistorySessions
// stands for no audience.
func TestClientMixFromHistory(t *testing.T) {
	spec, ok := clientMixFromHistory(map[PlaybackDevice]int{
		{Type: "Evey TV", Name: "Evey TV"}: 12, {Type: "Evey TV", Name: "TV Browser"}: 5, {Type: "Evey Mobile", Name: "Pixel"}: 30,
		{Type: "Evey Web", Name: "Firefox"}: 8, {Type: "restore"}: 50, {}: 3,
	})
	if !ok || spec != "mobile=30,tv=12,tv-browser=5,web-chrome=8" {
		t.Fatalf("spec = %q ok = %v, want mobile=30,tv=12,tv-browser=5,web-chrome=8", spec, ok)
	}
	if _, ok := clientMixFromHistory(map[PlaybackDevice]int{{Type: "Evey TV"}: minHistorySessions - 1, {Type: "restore"}: 500}); ok {
		t.Fatal("a history of fewer playbacks than minHistorySessions was taken for an audience")
	}
	if _, ok := clientMixFromHistory(nil); ok {
		t.Fatal("no history was taken for an audience")
	}
}

// Every audience size holds the mix within one viewer per profile.
func TestClientForSlot_EveryPrefixHoldsTheMix(t *testing.T) {
	mix := []clientShare{{"tv", 50}, {"mobile", 30}, {"web-chrome", 20}}
	counts := map[string]int{}
	for slot := 1; slot <= 100; slot++ {
		counts[clientForSlot(mix, slot)]++
		for _, s := range mix {
			if d := float64(counts[s.profile]) - float64(s.weight)*float64(slot)/100; d > 1 || d < -1 {
				t.Fatalf("after %d viewers, %s has %d, %.1f expected", slot, s.profile, counts[s.profile], float64(s.weight)*float64(slot)/100)
			}
		}
	}
}

// A client viewer's master carries what its player declares, nothing forced.
func TestBuildMasterURL_ClientDeclaresItsPlayersCapabilities(t *testing.T) {
	q := func(profile string) url.Values {
		u, err := url.Parse(buildMasterURL(Config{BaseURL: "http://x"}, clientViewerType(profile), MediaCandidate{ID: "m"}, false, 0))
		if err != nil {
			t.Fatal(err)
		}
		return u.Query()
	}
	tv := q("tv")
	if tv.Get("hdr") != "1" || tv.Get("maxAudioChannels") != "6" || !strings.Contains(tv.Get("videoCodecs"), "hevc") || tv.Get("vd") != "" {
		t.Fatalf("tv query = %v", tv)
	}
	if b := q("tv-browser"); b.Get("hdr") != "" || strings.Contains(b.Get("videoCodecs"), "hevc") || b.Get("maxAudioChannels") != "2" || b.Get("wasmAudio") != "" {
		t.Fatalf("tv-browser query = %v", b)
	}
	web := q("web-chrome")
	if web.Get("hdr") != "" || web.Get("wasmAudio") != "1" || strings.Contains(web.Get("videoCodecs"), "hevc") || strings.Contains(web.Get("audioCodecs"), "ac3") {
		t.Fatalf("web-chrome query = %v", web)
	}
}

// A mix naming a profile the harness cannot play is refused before anything
// starts, rather than reported as an audience that never played.
func TestRunner_StartRefusesAnInvalidClientMix(t *testing.T) {
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	err := r.Start(context.Background(), Config{BaseURL: "http://127.0.0.1:1", Token: "t", Scenario: "clients:toaster=10"})
	if !errors.Is(err, ErrUnknownScenario) {
		t.Fatalf("Start = %v, want ErrUnknownScenario", err)
	}
}

// A history run plays the server's own mix and says so: the report names the
// resolved proportions, each viewer declares its profile, and the report counts
// the path the server chose for each kind of client.
func TestRunner_HistoryMixPlaysAndReportsEachClientsPath(t *testing.T) {
	st := newServerState()
	srv := httptest.NewServer(st.handler())
	defer srv.Close()

	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(6)}, zeroActive, testLogger())
	r.WithClientHistory(func(context.Context) (map[PlaybackDevice]int, error) {
		return map[PlaybackDevice]int{{Type: "Evey TV", Name: "Shield"}: 20, {Type: "Evey Web", Name: "Chrome"}: 20}, nil
	})
	cfg := Config{
		BaseURL: srv.URL, Token: "tkn", Scenario: "clients:history",
		MaxViewers: 4, RampEvery: 40 * time.Millisecond, FastHold: 300 * time.Millisecond, MeasureHold: 300 * time.Millisecond, MaxDuration: 20 * time.Second,
	}
	if err := r.Start(context.Background(), cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	report := waitForReport(t, r, 20*time.Second)
	if report.Scenario != "clients:tv=20,web-chrome=20" {
		t.Fatalf("scenario = %q, want the resolved history mix", report.Scenario)
	}
	for _, vt := range []string{"client:tv", "client:web-chrome"} {
		if report.PathsByType[vt][servedByMkvgo] == 0 {
			t.Fatalf("paths = %v, want %s viewers counted on the path the server served", report.PathsByType, vt)
		}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	hdr := 0
	for _, f := range st.fetches {
		if strings.HasSuffix(f.path, "/master.m3u8") && strings.Contains(f.rawQuery, "hdr=1") {
			hdr++
		}
	}
	if hdr == 0 {
		t.Fatal("no master was requested with the tv profile's hdr=1")
	}
}

// Too thin a history falls back to the default mix rather than a mix of one.
func TestRunner_ThinHistoryFallsBackToTheDefaultMix(t *testing.T) {
	r := New(&http.Client{}, fakeMediaLister{candidates: manyCandidates(2)}, zeroActive, testLogger())
	r.WithClientHistory(func(context.Context) (map[PlaybackDevice]int, error) {
		return map[PlaybackDevice]int{{Type: "Evey TV"}: 3}, nil
	})
	cfg, err := r.resolveClientMix(context.Background(), Config{Scenario: "clients:history"})
	if err != nil || cfg.Scenario != ScenarioClients+":"+DefaultClientMix {
		t.Fatalf("scenario = %q err = %v, want the default mix", cfg.Scenario, err)
	}
}
