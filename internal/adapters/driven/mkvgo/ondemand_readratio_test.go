package mkvgo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/gravity-zero/mkvgo/mp4"
)

type reported struct {
	st                mp4.HLSPlanStats
	served, resources int64
}

// A plan reading over three times what it served is reported once while in
// use, at the first readRatioMinServed step past the bar, and again with its
// final figures when it leaves the cache.
func TestServedWatch_NamesAPlanReadingOverTheBarOnce(t *testing.T) {
	var w servedWatch
	st := mp4.HLSPlanStats{}
	stats := func() mp4.HLSPlanStats { return st }
	var got []reported
	report := func(s mp4.HLSPlanStats, served, resources int64) { got = append(got, reported{s, served, resources}) }

	// Under the minimum served nothing is read, however bad the ratio.
	st.SourceReadBytes = 10 * readRatioMinServed
	w.count(readRatioMinServed/2, stats, report)
	if len(got) != 0 {
		t.Fatalf("reported before %d bytes served: %+v", readRatioMinServed, got)
	}
	// The step is crossed at 64 MiB served, 640 MiB read: named.
	w.count(readRatioMinServed/2, stats, report)
	if len(got) != 1 || got[0].served != readRatioMinServed || got[0].resources != 2 {
		t.Fatalf("got %+v, want one report at %d served after 2 resources", got, readRatioMinServed)
	}
	// Later steps still over the bar: not named again while in use.
	w.count(readRatioMinServed, stats, report)
	if len(got) != 1 {
		t.Fatalf("named twice while in use: %+v", got)
	}
	// Leaving the cache: the final figures, even back under the bar.
	st.SourceReadBytes = 2 * readRatioMinServed
	w.final(stats, report)
	if len(got) != 2 || got[1].served != 2*readRatioMinServed {
		t.Fatalf("final = %+v", got)
	}
}

// A plan at three times or under is never named, nor is one that served
// nothing when it leaves.
func TestServedWatch_AtOrUnderTheBarStaysQuiet(t *testing.T) {
	var w servedWatch
	st := mp4.HLSPlanStats{SourceReadBytes: readRatioFlag * readRatioMinServed}
	stats := func() mp4.HLSPlanStats { return st }
	n := 0
	report := func(mp4.HLSPlanStats, int64, int64) { n++ }
	w.count(readRatioMinServed, stats, report)
	w.final(stats, report)
	var idle servedWatch
	idle.final(func() mp4.HLSPlanStats { return mp4.HLSPlanStats{SourceReadBytes: 1 << 30} }, report)
	if n != 0 {
		t.Fatalf("%d reports for a plan at the bar or idle", n)
	}
}

// The log line carries what mkvgo needs to take the case up.
func TestLogOverRead_CarriesTheCase(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	st := mp4.HLSPlanStats{SourceReadBytes: 400, WindowBuilds: 7, FarLookups: 5, FarHits: 4, KeptSegments: 30, KeptFallbacks: 1, StreamedSegments: 2, StreamFallbacks: 3}
	logOverRead(logger, "/media/a.mkv", st, 100, 12, true)
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"path": "/media/a.mkv", "read_bytes": 400.0, "served_bytes": 100.0, "read_ratio_pct": 400.0,
		"resources_served": 12.0, "segments_opened": 35.0, "window_builds": 7.0, "far_lookups": 5.0, "far_hits": 4.0,
		"kept_segments": 30.0, "kept_fallbacks": 1.0, "final": true}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v (%s)", k, line[k], v, buf.String())
		}
	}
}

// fakeStream is a streamed resource writing n bytes per call.
type fakeStream struct{}

func (fakeStream) ContentType() string { return "video/mp4" }
func (fakeStream) Size() int64         { return 1000 }
func (fakeStream) ETag() string        { return `W/"x"` }
func (fakeStream) Bytes() []byte       { return nil }
func (fakeStream) Close() error        { return nil }
func (fakeStream) WriteRange(_ context.Context, w io.Writer, _, n int64) (int64, error) {
	m, err := w.Write(make([]byte, n))
	return int64(m), err
}

// What a streamed segment wrote to its client is what the plan counts served.
func TestCountedResource_CountsWhatWentOut(t *testing.T) {
	p := &ondemandPlan{}
	c := &countedResource{OnDemandResource: fakeStream{}, plan: p}
	if _, err := c.WriteRange(context.Background(), io.Discard, 0, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := c.WriteRange(context.Background(), io.Discard, 300, 200); err != nil {
		t.Fatal(err)
	}
	if b, r := p.watch.bytes.Load(), p.watch.resources.Load(); b != 500 || r != 2 {
		t.Fatalf("served %d bytes over %d resources, want 500 over 2", b, r)
	}
}
