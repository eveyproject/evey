package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// streamingConcat is streamingPlan as a concatenated session.
type streamingConcat struct{ *streamingPlan }

func (streamingConcat) NumParts() int { return 2 }

func newConcatStreamingServer(t *testing.T, plan *streamingPlan) *httptest.Server {
	t.Helper()
	h := &hlsVODServe{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serveConcatResourceData(w, r, streamingConcat{plan}, strings.TrimPrefix(r.URL.Path, "/"), trace.SpanFromContext(r.Context()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A concat session's media segment streams like a single episode's, marked as
// the concat's; its playlist still comes whole off Resource.
func TestServeConcatResource_StreamsSegmentsPlaylistsWhole(t *testing.T) {
	seg := []byte(strings.Repeat("c", 50_000))
	plan := &streamingPlan{segment: seg, fakeOnDemandPlan: fakeOnDemandPlan{resources: map[string][]byte{
		"playlist.m3u8": []byte("#EXTM3U\n#EXT-X-DISCONTINUITY\n"),
	}}}
	srv := newConcatStreamingServer(t, plan)

	res, err := http.Get(srv.URL + "/seg00001.m4s")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != string(seg) {
		t.Fatalf("segment: status %d, %d bytes", res.StatusCode, len(body))
	}
	if got := res.Header.Get("X-Evey-Served-By"); got != "mkvgo-concat" {
		t.Fatalf("X-Evey-Served-By = %q, want mkvgo-concat", got)
	}
	if got := res.Header.Get("Content-Length"); got != "50000" {
		t.Fatalf("Content-Length = %q", got)
	}
	if n := plan.closedCount(); n != 1 {
		t.Fatalf("handle closed %d times, want 1", n)
	}

	res, err = http.Get(srv.URL + "/playlist.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "EXT-X-DISCONTINUITY") {
		t.Fatalf("playlist: status %d body %q", res.StatusCode, body)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("playlist Cache-Control = %q", got)
	}
	for _, n := range plan.openedNames() {
		if n == "playlist.m3u8" {
			t.Fatal("the playlist went through Open, not Resource")
		}
	}
}

// A source read error midway through a concat segment breaks the transfer.
func TestServeConcatResource_WriteErrorCutsTheConnection(t *testing.T) {
	seg := []byte(strings.Repeat("x", 100_000))
	srv := newConcatStreamingServer(t, &streamingPlan{segment: seg, failAfter: 1000})
	res, err := http.Get(srv.URL + "/seg00001.m4s")
	if err != nil {
		return // dropped before the status line: a broken transfer
	}
	body, readErr := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if readErr == nil {
		t.Fatalf("read %d of %d bytes with no error: a truncated segment passed as whole", len(body), len(seg))
	}
}
