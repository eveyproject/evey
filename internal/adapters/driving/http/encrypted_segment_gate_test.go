package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestClientSegmentGate_RefusesPastTheLimitPerClient(t *testing.T) {
	g := newClientSegmentGate(2)
	r1, ok1 := g.acquire("p:a")
	_, ok2 := g.acquire("p:a")
	_, ok3 := g.acquire("p:a")
	if !ok1 || !ok2 || ok3 {
		t.Fatalf("acquire = %v %v %v, want two slots then a refusal", ok1, ok2, ok3)
	}
	if _, ok := g.acquire("p:b"); !ok {
		t.Fatal("another client is refused by the first one's slots")
	}
	r1()
	r1() // a second release frees nothing more
	if _, ok := g.acquire("p:a"); !ok {
		t.Fatal("a released slot is not given back")
	}
	if _, ok := g.acquire("p:a"); ok {
		t.Fatal("a double release gave a slot twice")
	}
}

func TestIsPlanMediaSegment(t *testing.T) {
	for name, want := range map[string]bool{
		"seg00001.m4s": true, "seg_a1_7.m4s": true, "seg_a12_10345.m4s": true,
		"index.m3u8": false, "master.m3u8": false, "init.mp4": false, "init_a1.mp4": false,
		"sub3_4.vtt": false, "key": false, "seg1.m4s": false, "seg00001.m4s.bak": false,
	} {
		if got := isPlanMediaSegment(name); got != want {
			t.Errorf("isPlanMediaSegment(%q) = %v, want %v", name, got, want)
		}
	}
}

// heldPlan holds a media segment's build until the test lets it go, the way
// an encrypted segment is built whole before its first byte goes out.
type heldPlan struct {
	fakeOnDemandPlan
	entered chan struct{}
	release chan struct{}
}

func (p *heldPlan) Resource(ctx context.Context, name string) ([]byte, string, error) {
	if isPlanMediaSegment(name) {
		p.entered <- struct{}{}
		<-p.release
		return []byte("encrypted"), "video/iso.segment", nil
	}
	return p.fakeOnDemandPlan.Resource(ctx, name)
}

func TestServePlanResource_EncryptedSegmentsLimitedPerClient(t *testing.T) {
	plan := &heldPlan{
		fakeOnDemandPlan: fakeOnDemandPlan{resources: map[string][]byte{"index.m3u8": []byte("#EXTM3U\n")}},
		entered:          make(chan struct{}, 4),
		release:          make(chan struct{}),
	}
	h := &hlsVODServe{encryptedSegments: newClientSegmentGate(1)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.servePlanResourceData(w, r, "media-1", plan, nil, strings.TrimPrefix(r.URL.Path, "/"), trace.SpanFromContext(r.Context()))
	}))
	defer srv.Close()
	get := func(name string) (int, string, string) {
		res, err := http.Get(srv.URL + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Retry-After"), string(body)
	}

	first := make(chan int)
	go func() { code, _, _ := get("seg00001.m4s"); first <- code }()
	<-plan.entered

	// The client's one slot is held: the next segment, video or audio, is
	// refused with what players retry, not built.
	if code, retry, _ := get("seg_a1_1.m4s"); code != http.StatusServiceUnavailable || retry == "" {
		t.Fatalf("second segment: status %d Retry-After %q, want 503 with Retry-After", code, retry)
	}
	// A playlist is not an encrypted segment and is never limited.
	if code, _, body := get("index.m3u8"); code != http.StatusOK || !strings.HasPrefix(body, "#EXTM3U") {
		t.Fatalf("playlist: status %d body %q", code, body)
	}

	close(plan.release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("held segment: status %d", code)
	}
	// Its slot went back with the response.
	if code, _, body := get("seg00002.m4s"); code != http.StatusOK || body != "encrypted" {
		t.Fatalf("after release: status %d body %q", code, body)
	}
}

func TestServePlanResource_NoGateWhenEncryptionIsOff(t *testing.T) {
	plan := &heldPlan{entered: make(chan struct{}, 4), release: make(chan struct{})}
	close(plan.release)
	h := &hlsVODServe{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.servePlanResourceData(w, r, "media-1", plan, nil, strings.TrimPrefix(r.URL.Path, "/"), trace.SpanFromContext(r.Context()))
	}))
	defer srv.Close()
	for range 3 {
		res, err := http.Get(srv.URL + "/seg00001.m4s")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d without encryption", res.StatusCode)
		}
	}
}
