package mkvgo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mp4"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

type reported struct {
	st        mp4.HLSPlanStats
	resources int64
}

// served is a plan's counters having served n bytes through Resource.
func served(n, read int64) mp4.HLSPlanStats {
	return mp4.HLSPlanStats{ServedBytes: n, WalkReadBytes: read, SourceReadBytes: read}
}

// A plan reading over three times what it served is reported once while in
// use, at the first readRatioMinServed step Evey hands out past the bar, and
// again with its final figures when it leaves the cache. The ratio is read on
// mkvgo's own count, Resource and Open together.
func TestServedWatch_NamesAPlanReadingOverTheBarOnce(t *testing.T) {
	w := servedWatch{}
	st := mp4.HLSPlanStats{}
	stats := func() mp4.HLSPlanStats { return st }
	var got, damaged []reported
	report := func(s mp4.HLSPlanStats, resources int64) { got = append(got, reported{s, resources}) }
	damage := func(s mp4.HLSPlanStats, resources int64) { damaged = append(damaged, reported{s, resources}) }

	// Under the minimum served nothing is read, however bad the ratio.
	st = served(readRatioMinServed/2, 10*readRatioMinServed)
	w.count(readRatioMinServed/2, stats, report, damage)
	if len(got) != 0 {
		t.Fatalf("reported before %d bytes served: %+v", readRatioMinServed, got)
	}
	// The step is crossed at 64 MiB served (half streamed), 640 MiB read: named.
	st = served(readRatioMinServed/2, 10*readRatioMinServed)
	st.StreamedBytes = readRatioMinServed / 2
	w.count(readRatioMinServed/2, stats, report, damage)
	if len(got) != 1 || planServed(got[0].st) != readRatioMinServed || got[0].resources != 2 {
		t.Fatalf("got %+v, want one report at %d served after 2 resources", got, readRatioMinServed)
	}
	// Later steps still over the bar: not named again while in use.
	st.ServedBytes += readRatioMinServed
	w.count(readRatioMinServed, stats, report, damage)
	if len(got) != 1 {
		t.Fatalf("named twice while in use: %+v", got)
	}
	// Leaving the cache: the final figures, even back under the bar.
	st.WalkReadBytes = 2 * readRatioMinServed
	w.final(stats, report, damage)
	if len(got) != 2 || planServed(got[1].st) != 2*readRatioMinServed {
		t.Fatalf("final = %+v", got)
	}
	if len(damaged) != 0 {
		t.Fatalf("an undamaged plan named damaged: %+v", damaged)
	}
}

// A plan at three times or under is never named, nor is one that served
// nothing when it leaves.
func TestServedWatch_AtOrUnderTheBarStaysQuiet(t *testing.T) {
	w := servedWatch{}
	st := served(readRatioMinServed, readRatioFlag*readRatioMinServed)
	stats := func() mp4.HLSPlanStats { return st }
	n := 0
	report := func(mp4.HLSPlanStats, int64) { n++ }
	w.count(readRatioMinServed, stats, report, report)
	w.final(stats, report, report)
	idle := servedWatch{}
	idle.final(func() mp4.HLSPlanStats { return mp4.HLSPlanStats{WalkReadBytes: 1 << 30, DamageSkips: 1} }, report, report)
	if n != 0 {
		t.Fatalf("%d reports for a plan at the bar or idle", n)
	}
}

// The ratio is read on the walks: a plan whose segments were copied again from
// the page cache (a local file since mkvgo 0.57, CopyReadBytes) is neither
// named nor counted over the bar, however much the total says it read. Its
// damaged regions are still named.
func TestServedWatch_SegmentCopiesDoNotCount(t *testing.T) {
	var w servedWatch
	st := mp4.HLSPlanStats{ServedBytes: readRatioMinServed, WalkReadBytes: readRatioMinServed + readRatioMinServed/3,
		CopyReadBytes: 10 * readRatioMinServed, SourceReadBytes: 11*readRatioMinServed + readRatioMinServed/3, DamageSkips: 1}
	stats := func() mp4.HLSPlanStats { return st }
	n, damaged := 0, 0
	report := func(mp4.HLSPlanStats, int64) { n++ }
	damage := func(mp4.HLSPlanStats, int64) { damaged++ }
	w.count(readRatioMinServed, stats, report, damage)
	w.final(stats, report, damage)
	if n != 0 || w.countedOver(overRead(st)) {
		t.Fatalf("a plan walking 1.33x its output was named %d times or counted over the bar", n)
	}
	if damaged != 1 {
		t.Fatalf("its damaged region named %d times, want once", damaged)
	}
}

// A plan that passed over a damaged region names it once, at the first step
// that sees it or when it leaves the cache; a refused region too.
func TestServedWatch_NamesADamagedPlanOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   mp4.HLSPlanStats
	}{
		{"skipped", mp4.HLSPlanStats{ServedBytes: readRatioMinServed, DamageSkips: 1}},
		{"refused", mp4.HLSPlanStats{ServedBytes: readRatioMinServed, DamageRefusals: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var w servedWatch
			stats := func() mp4.HLSPlanStats { return tc.st }
			n := 0
			damage := func(mp4.HLSPlanStats, int64) { n++ }
			none := func(mp4.HLSPlanStats, int64) {}
			w.count(readRatioMinServed, stats, none, damage)
			w.count(readRatioMinServed, stats, none, damage)
			w.final(stats, none, damage)
			if n != 1 {
				t.Fatalf("named %d times, want once", n)
			}
		})
	}
	// Seen only at the end: named when it leaves.
	var w servedWatch
	n := 0
	w.count(1, func() mp4.HLSPlanStats { return mp4.HLSPlanStats{} }, nil, nil)
	w.final(func() mp4.HLSPlanStats { return mp4.HLSPlanStats{DamageSkips: 2} }, func(mp4.HLSPlanStats, int64) {}, func(mp4.HLSPlanStats, int64) { n++ })
	if n != 1 {
		t.Fatalf("named %d times on leaving, want once", n)
	}
}

// The log line carries what mkvgo needs to take the case up.
func TestLogOverRead_CarriesTheCase(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	st := mp4.HLSPlanStats{SourceReadBytes: 900, WalkReadBytes: 400, CopyReadBytes: 500, ServedBytes: 60, StreamedBytes: 40, ServedSegments: 35, WindowBuilds: 7, FarLookups: 5, FarHits: 4, KeptSegments: 30, KeptFallbacks: 1, StreamedSegments: 2, StreamFallbacks: 3}
	logOverRead(logger, "/media/a.mkv", st, 12, true)
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"path": "/media/a.mkv", "walk_read_bytes": 400.0, "copy_read_bytes": 500.0, "served_bytes": 100.0, "read_ratio_pct": 400.0,
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

// mkvgo's refusals reach the port as its errors, mkvgo's own kept in the
// chain: a replaced source, a window inside a damaged region; anything else
// passes through.
func TestSourceChangedErr_MapsMkvgoRefusals(t *testing.T) {
	damage := fmt.Errorf("window 287: %w", &reader.DamageError{Offset: 1260397865, Resume: -1, Cause: io.ErrUnexpectedEOF})
	got := sourceChangedErr(damage)
	if !errors.Is(got, apptranscoding.ErrSourceDamaged) || !errors.Is(got, reader.ErrDamagedRegion) || errors.Is(got, apptranscoding.ErrSourceChanged) {
		t.Fatalf("damaged region mapped to %v", got)
	}
	if got := sourceChangedErr(mp4.ErrSourceChanged); !errors.Is(got, apptranscoding.ErrSourceChanged) || errors.Is(got, apptranscoding.ErrSourceDamaged) {
		t.Fatalf("replaced source mapped to %v", got)
	}
	if got := sourceChangedErr(io.ErrUnexpectedEOF); !errors.Is(got, io.ErrUnexpectedEOF) || errors.Is(got, apptranscoding.ErrSourceDamaged) || errors.Is(got, apptranscoding.ErrSourceChanged) {
		t.Fatalf("other error mapped to %v", got)
	}
	if sourceChangedErr(nil) != nil {
		t.Fatal("nil mapped to an error")
	}
}

// "plans_read_over_ratio" is diffed between two WindowStats snapshots by the
// load test, so a plan counted over the bar stays counted when its ratio falls
// back under it as it serves more: a live state there logged negative runs.
func TestServedWatch_CountedOverOnlyGrows(t *testing.T) {
	var w servedWatch
	if w.countedOver(false) {
		t.Fatal("a plan never over the bar is not counted")
	}
	if !w.countedOver(true) {
		t.Fatal("a plan over the bar is counted")
	}
	if !w.countedOver(false) {
		t.Fatal("a plan counted once stays counted after falling back under the bar")
	}
	var named servedWatch
	named.flagged.Store(true)
	if !named.countedOver(false) {
		t.Fatal("a plan already named over the bar is counted")
	}
}
