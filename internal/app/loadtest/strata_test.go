package loadtest

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// stratifiedCandidates is a library on three disks holding 48/40/12 % of the
// titles, each disk spread over four bitrate quarters.
func stratifiedCandidates() ([]MediaCandidate, []string) {
	var cands []MediaCandidate
	var disks []string
	add := func(disk string, n int) {
		for i := range n {
			cands = append(cands, MediaCandidate{ID: fmt.Sprintf("%s-%d", disk, i), DurationSec: 7200, BitrateBps: int64(4+i%4*10) * 1_000_000})
			disks = append(disks, disk)
		}
	}
	add("/mnt/disk_1", 120)
	add("/mnt/disk_2", 100)
	add("/mnt/disk_3", 30)
	return cands, disks
}

func liveShareOf(l *lane, disk string) float64 {
	total, n := 0, 0
	for k, c := range l.live {
		total += c
		if d, _ := splitStratum(k); d == disk {
			n += c
		}
	}
	return float64(n) / float64(max(total, 1))
}

// Every audience size holds each disk to its share of the eligible titles,
// through growth, shrinkage by the oldest or the youngest, and refills: the
// pool fills the stratum furthest behind in the LIVE audience.
func TestLane_EveryAudienceSizeHoldsEachDiskToItsShare(t *testing.T) {
	cands, disks := stratifiedCandidates()
	drawn, _, _, classes := representativeDrawOnDisks(cands, disks, 7)
	pool := newLanedPool(cands, map[viewerType]drawnLane{viewerTypeMkvgo: {titles: drawn, classes: classes}}, time.Hour)
	l := pool.laneFor(viewerTypeMkvgo)
	var live []assignment
	check := func(step string) {
		t.Helper()
		n := float64(len(live))
		for disk, want := range map[string]float64{"/mnt/disk_1": 0.48, "/mnt/disk_2": 0.40, "/mnt/disk_3": 0.12} {
			got := liveShareOf(l, disk)
			if d := (got - want) * n; d > 1.01 || d < -1.01 {
				t.Fatalf("%s, %d viewers: %s holds %.2f, want %.2f within one viewer", step, len(live), disk, got, want)
			}
		}
	}
	take := func(n int) {
		for range n {
			asg, ok := pool.takeFor(viewerTypeMkvgo)
			if !ok {
				t.Fatal("pool exhausted")
			}
			live = append(live, asg)
		}
	}
	take(141)
	check("after the climb to 141")
	// The climb sheds the youngest down to 101, then the grid churns the oldest.
	for len(live) > 101 {
		a := live[len(live)-1]
		live = live[:len(live)-1]
		pool.release(viewerTypeMkvgo, a.stratum)
	}
	check("after shedding to 101")
	for round := range 10 {
		for range 3 {
			a := live[0]
			live = live[1:]
			pool.release(viewerTypeMkvgo, a.stratum)
		}
		take(3)
		check(fmt.Sprintf("after churn %d", round))
	}
}

// A title retired mid-run is replaced from the same stratum, and a replay
// stays in its title's stratum.
func TestLane_ReplacementsAndReplaysStayInTheirStratum(t *testing.T) {
	cands, disks := stratifiedCandidates()
	drawn, _, _, classes := representativeDrawOnDisks(cands, disks, 3)
	pool := newLanedPool(cands, map[viewerType]drawnLane{viewerTypeMkvgo: {titles: drawn, classes: classes}}, time.Hour)
	var held []assignment
	for range 20 {
		a, _ := pool.takeFor(viewerTypeMkvgo)
		held = append(held, a)
	}
	gone := held[7]
	pool.markIneligibleFor(viewerTypeMkvgo, gone.media.ID)
	pool.release(viewerTypeMkvgo, gone.stratum)
	repl, ok := pool.takeFor(viewerTypeMkvgo)
	if !ok || repl.stratum != gone.stratum || repl.media.ID == gone.media.ID {
		t.Fatalf("replacement %q (%s), want another title of %s", repl.media.ID, repl.stratum, gone.stratum)
	}
	// Exhaust round 0 everywhere: every replay keeps the stratum of its title.
	strata := map[string]string{}
	for i, c := range drawn {
		strata[c.ID] = classes[i]
	}
	for range 600 {
		a, ok := pool.takeFor(viewerTypeMkvgo)
		if !ok {
			break
		}
		if strata[a.media.ID] != a.stratum {
			t.Fatalf("title %s handed out in %s, drawn in %s", a.media.ID, a.stratum, strata[a.media.ID])
		}
	}
}

// The draw's strata follow the eligible titles: a disk whose titles the server
// will not package for mkvgo gets no place, and the others keep theirs.
func TestRunner_IneligibleTitlesAreKeptOutBeforeTheDraw(t *testing.T) {
	st := newServerState()
	st.servedByFn = func(mediaID, resource string, _ int) bool {
		return resource != "master" || mediaID != "m1"
	}
	r := New(nil, fakeMediaLister{}, zeroActive, testLogger())
	ts := httptest.NewServer(st.handler())
	defer ts.Close()
	srv := ts.URL
	var dropped []string
	var forgotten []string
	r.WithPlanDrop(func(paths []string) { dropped = paths })
	r.WithPlaybackForget(func(ids []string) { forgotten = ids })
	got := r.mkvgoEligible(context.Background(), Config{BaseURL: srv, Token: "tkn"}, manyCandidates(3))
	if len(got) != 2 || got[0].ID != "m0" || got[1].ID != "m2" {
		t.Fatalf("eligible = %+v, want m0 and m2", got)
	}
	if len(dropped) != 2 {
		t.Fatalf("plans dropped for %v, want the two eligible titles", dropped)
	}
	if len(forgotten) != 3 {
		t.Fatalf("activity forgotten for %v, want every probed title", forgotten)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.masterHits != 0 || len(st.fetches) != 0 {
		t.Fatalf("the probes counted as viewers: %d master hits, %d fetches", st.masterHits, len(st.fetches))
	}
}

// What gives out first, and the advice that follows from it.
func TestLimiterOf(t *testing.T) {
	devices := []DeviceMapping{{Location: "/mnt/disk_1", Device: "sda"}, {Location: "/mnt/disk_2", Device: "sdc"}, {Location: "/mnt/disk_3", Device: "sdd"}}
	busy := ResourceStats{Disks: []DiskStat{{Device: "sda", UtilPctAvg: 90}, {Device: "sdc", UtilPctAvg: 25}, {Device: "sdd", UtilPctAvg: 3}}}
	sat := DiskVerdict{Device: "sda", Saturated: true}
	for _, tc := range []struct {
		name                      string
		stop                      StopReason
		detail                    string
		res                       ResourceStats
		v                         DiskVerdict
		proxy                     bool
		limiter, location, advice string
	}{
		{"saturated disk with room elsewhere", StopSLOBreach, "", busy, sat, false, LimiterDisk, "/mnt/disk_1", AdviceSpreadTitles},
		{"every disk busy: no advice", StopSLOBreach, "", ResourceStats{Disks: []DiskStat{{Device: "sda", UtilPctAvg: 90}, {Device: "sdc", UtilPctAvg: 85}, {Device: "sdd", UtilPctAvg: 80}}}, sat, false, LimiterDisk, "/mnt/disk_1", ""},
		{"cpu peak", StopSLOBreach, "", ResourceStats{CPUPercentPeak: 95}, DiskVerdict{}, false, LimiterCPU, "", ""},
		{"cpu verdict", StopCPUSaturated, "", ResourceStats{}, DiskVerdict{}, false, LimiterCPU, "", ""},
		{"memory", StopMemorySaturated, "", ResourceStats{}, DiskVerdict{}, false, LimiterMemory, "", ""},
		{"server errors", StopServerFailing, "master playlist fetch failed (status=502, err=<nil>)", ResourceStats{}, DiskVerdict{}, false, LimiterServer, "", ""},
		{"through the proxy, nothing saturated", StopSLOBreach, "", ResourceStats{}, DiskVerdict{Device: "sda"}, true, LimiterProxy, "", ""},
		{"nothing saturated", StopSLOBreach, "", ResourceStats{}, DiskVerdict{Device: "sda"}, false, LimiterUnknown, "", ""},
		{"operator ceiling", StopMaxViewers, "", ResourceStats{}, DiskVerdict{}, false, LimiterNone, "", ""},
		{"cancelled", StopCancelled, "", ResourceStats{}, DiskVerdict{}, false, "", "", ""},
	} {
		l, loc, adv := limiterOf(tc.stop, tc.detail, tc.res, tc.v, tc.proxy, devices)
		if l != tc.limiter || loc != tc.location || adv != tc.advice {
			t.Errorf("%s: got (%q, %q, %q), want (%q, %q, %q)", tc.name, l, loc, adv, tc.limiter, tc.location, tc.advice)
		}
	}
}
