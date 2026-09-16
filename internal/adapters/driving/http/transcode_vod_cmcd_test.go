package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	appqoe "github.com/evey-project/evey/internal/app/qoe"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

type recordingQoESink struct{ events []appqoe.Event }

func (s *recordingQoESink) Record(evt appqoe.Event)     { s.events = append(s.events, evt) }
func (s *recordingQoESink) Close(context.Context) error { return nil }

// CMCD ingest is a side channel on the segment path: nothing is recorded
// without a sink or without a CMCD payload, and a beacon lands with the
// caller's identity, its position in the stream and the parsed telemetry.
func TestIngestCMCD(t *testing.T) {
	sink := &recordingQoESink{}
	h := &hlsVODServe{}

	plain := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v0/seg_3.m4s", nil)
	h.ingestCMCD(plain, "m1", 0, 3, false, false, false)
	h.qoeSink = sink
	h.ingestCMCD(plain, "m1", 0, 3, false, false, false)
	if len(sink.events) != 0 {
		t.Fatalf("no sink or no CMCD payload must record nothing, got %d", len(sink.events))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v1/seg_7.m4s?CMCD=bl%3D2500%2Cbr%3D6000%2Cbs%2Csid%3D%22sess-1%22", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Firefox/130.0")
	req.RemoteAddr = "192.168.1.20:51234"
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	h.ingestCMCD(req, "m1", 1, 7, false, false, false)
	if len(sink.events) != 1 {
		t.Fatalf("one beacon = one event, got %d", len(sink.events))
	}
	e := sink.events[0]
	if e.UserID != "u1" || e.MediaItemID != "m1" || e.VariantIdx != 1 || e.SegmentIdx != 7 {
		t.Errorf("identity/position: %+v", e)
	}
	if e.Payload.SessionID != "sess-1" || e.Payload.BufferLengthMs != 2500 || e.Payload.EncodedKbps != 6000 || !e.Payload.BufferStarved {
		t.Errorf("payload: %+v", e.Payload)
	}
	if e.RemoteIP != "192.168.1.20" {
		t.Errorf("remote ip = %q", e.RemoteIP)
	}
	if e.DeviceClass == "" {
		t.Error("device class derived from the user agent")
	}

	// Headers are the other transport; an anonymous (share link) play has no user.
	hdr := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
	hdr.Header.Set("CMCD-Session", `sid="sess-2"`)
	hdr.Header.Set("CMCD-Status", "bs")
	h.ingestCMCD(hdr, "m1", 0, 0, false, true, false)
	if len(sink.events) != 2 || sink.events[1].UserID != "" || !sink.events[1].IsManifest || sink.events[1].Payload.SessionID != "sess-2" {
		t.Errorf("header beacon: %+v", sink.events)
	}
}
