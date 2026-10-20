package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// statPlan answers Stat from a fixed table and records whether Stat was asked.
type statPlan struct {
	size   int64
	etag   string
	ct     string
	known  bool
	err    error
	called bool
}

func (p *statPlan) Stat(_ context.Context, _ string) (int64, string, string, bool, error) {
	p.called = true
	return p.size, p.etag, p.ct, p.known, p.err
}

func statRequest(method, name string, header map[string]string) *http.Request {
	r := httptest.NewRequest(method, "/hls/m/"+name, nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	return r
}

// A HEAD is answered from Stat with the size and ETag a GET would send, and
// no body; a revalidation that matches is a 304. Neither opens the resource.
func TestAnswerPlanResourceByStat_HeadAndRevalidation(t *testing.T) {
	plan := &statPlan{size: 4242, etag: `W/"abc"`, ct: "video/iso.segment", known: true}

	w := httptest.NewRecorder()
	if !answerPlanResourceByStat(w, statRequest(http.MethodHead, "od_v_seg00003.m4s", nil), plan, "od_v_seg00003.m4s", "mkvgo") {
		t.Fatal("a HEAD on a segment must be answered from Stat")
	}
	if w.Code != http.StatusOK || w.Header().Get("Content-Length") != "4242" || w.Header().Get("ETag") != `W/"abc"` ||
		w.Header().Get("Content-Type") != "video/iso.segment" || w.Body.Len() != 0 {
		t.Fatalf("HEAD: code %d, headers %v, body %d bytes", w.Code, w.Header(), w.Body.Len())
	}

	w = httptest.NewRecorder()
	if !answerPlanResourceByStat(w, statRequest(http.MethodGet, "seg.m4s", map[string]string{"If-None-Match": `"abc"`}), plan, "seg.m4s", "mkvgo") {
		t.Fatal("a matching revalidation must be answered from Stat")
	}
	if w.Code != http.StatusNotModified {
		t.Fatalf("revalidation: code %d, want 304", w.Code)
	}
}

// Everything Stat cannot answer exactly goes the normal way, untouched.
func TestAnswerPlanResourceByStat_LeavesTheRestToOpen(t *testing.T) {
	cases := []struct {
		name   string
		req    *http.Request
		plan   any
		asked  bool
		reason string
	}{
		{"seg.m4s", statRequest(http.MethodGet, "seg.m4s", nil), &statPlan{known: true}, false, "a plain GET"},
		{"seg.m4s", statRequest(http.MethodGet, "seg.m4s", map[string]string{"If-None-Match": `"old"`}), &statPlan{etag: `W/"new"`, known: true}, true, "a revalidation that does not match"},
		{"seg.m4s", statRequest(http.MethodHead, "seg.m4s", map[string]string{"Range": "bytes=0-9"}), &statPlan{known: true}, false, "a HEAD with a Range"},
		{"master.m3u8", statRequest(http.MethodHead, "master.m3u8", nil), &statPlan{known: true}, false, "a playlist Evey rewrites"},
		{"sub2_0.vtt", statRequest(http.MethodHead, "sub2_0.vtt", nil), &statPlan{known: true}, false, "a subtitle Evey sanitises"},
		{"seg.m4s", statRequest(http.MethodHead, "seg.m4s", nil), &statPlan{known: false}, true, "a resource only Open can size"},
		{"seg.m4s", statRequest(http.MethodHead, "seg.m4s", nil), &statPlan{err: errors.New("boom")}, true, "a Stat that fails"},
		{"seg.m4s", statRequest(http.MethodHead, "seg.m4s", nil), struct{}{}, false, "a plan without Stat"},
	}
	for _, c := range cases {
		t.Run(c.reason, func(t *testing.T) {
			w := httptest.NewRecorder()
			if answerPlanResourceByStat(w, c.req, c.plan, c.name, "mkvgo") {
				t.Fatal("answered, want the normal path")
			}
			if len(w.Header()) != 0 || w.Code != http.StatusOK || w.Body.Len() != 0 {
				t.Fatalf("the response was touched: code %d, headers %v", w.Code, w.Header())
			}
			if sp, ok := c.plan.(*statPlan); ok && sp.called != c.asked {
				t.Fatalf("Stat asked = %v, want %v", sp.called, c.asked)
			}
		})
	}
}

// A name the plan does not serve and a damaged window are 404s; a source that
// could not be read is a 502 a player retries.
func TestPlanResourceErrorStatus(t *testing.T) {
	for err, want := range map[error]int{
		fmt.Errorf("wrap: %w", apptranscoding.ErrNoResource):    http.StatusNotFound,
		fmt.Errorf("wrap: %w", apptranscoding.ErrSourceDamaged): http.StatusNotFound,
		fmt.Errorf("wrap: %w", apptranscoding.ErrSourceChanged): http.StatusNotFound,
		errors.New("read /media/x.mkv: input/output error"):     http.StatusBadGateway,
	} {
		if got := planResourceErrorStatus(err); got != want {
			t.Errorf("%v: status %d, want %d", err, got, want)
		}
	}
}
