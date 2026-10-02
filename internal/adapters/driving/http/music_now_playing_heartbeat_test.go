package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

func heartbeatReq(body string, claims *domainuser.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/music/now-playing", strings.NewReader(body))
	if claims != nil {
		r = withHTTPClaims(r, claims)
	}
	return r
}

// The heartbeat is what puts a music session in the admin "now playing"
// panel and carries the admin's remote command back to the player.
func TestNowPlayingHeartbeat(t *testing.T) {
	reg := appmusic.NewNowPlayingRegistry()
	h := &musicHandler{nowPlaying: reg}
	claims := &domainuser.Claims{UserID: "u1", Username: "alice", ProfileID: "p1"}

	// No registry wired, or no profile on the claims: silently a no-op.
	w := httptest.NewRecorder()
	(&musicHandler{}).nowPlayingHeartbeat(w, heartbeatReq(`{"sessionId":"s1"}`, claims))
	if w.Code != http.StatusNoContent {
		t.Errorf("no registry: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.nowPlayingHeartbeat(w, heartbeatReq(`{"sessionId":"s1"}`, &domainuser.Claims{UserID: "u1"}))
	if w.Code != http.StatusNoContent || len(reg.List()) != 0 {
		t.Errorf("profile-less caller must not register a session: %d, %d sessions", w.Code, len(reg.List()))
	}

	// A body without a session id is refused.
	w = httptest.NewRecorder()
	h.nowPlayingHeartbeat(w, heartbeatReq(`{"trackId":"t1"}`, claims))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing sessionId: %d", w.Code)
	}

	// A live heartbeat registers the session under the caller's profile and
	// answers with an empty command.
	w = httptest.NewRecorder()
	h.nowPlayingHeartbeat(w, heartbeatReq(`{"sessionId":"s1","trackId":"t1","deviceName":"Kitchen","positionMs":4200}`, claims))
	if w.Code != http.StatusOK {
		t.Fatalf("heartbeat: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp["remoteCommand"] != "" {
		t.Errorf("first heartbeat: %v %v", resp, err)
	}
	live := reg.List()
	if len(live) != 1 || live[0].ProfileID != "p1" || live[0].Username != "alice" || live[0].TrackID != "t1" || live[0].DeviceName != "Kitchen" || live[0].PositionMs != 4200 {
		t.Errorf("registered session = %+v", live)
	}

	// The admin's queued command rides the next heartbeat, once.
	if !reg.SetCommand("s1", "pause") {
		t.Fatal("session must be known")
	}
	w = httptest.NewRecorder()
	h.nowPlayingHeartbeat(w, heartbeatReq(`{"sessionId":"s1","trackId":"t1","isPaused":false}`, claims))
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["remoteCommand"] != "pause" {
		t.Errorf("command not delivered: %v", resp)
	}
	w = httptest.NewRecorder()
	h.nowPlayingHeartbeat(w, heartbeatReq(`{"sessionId":"s1","trackId":"t1","isPaused":true}`, claims))
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["remoteCommand"] != "" {
		t.Errorf("a command is consumed once: %v", resp)
	}

	// stopped=true drops the session from the panel immediately.
	w = httptest.NewRecorder()
	h.nowPlayingHeartbeat(w, heartbeatReq(`{"sessionId":"s1","stopped":true}`, claims))
	if w.Code != http.StatusNoContent || len(reg.List()) != 0 {
		t.Errorf("stop: %d, %d sessions left", w.Code, len(reg.List()))
	}
}

func waitCommandReq(sessionID string, claims *domainuser.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/music/nowplaying/"+sessionID+"/command", nil)
	r.SetPathValue("sessionId", sessionID)
	if claims != nil {
		r = withHTTPClaims(r, claims)
	}
	return r
}

// The command poll answers the moment the admin queues a command, and only to
// the profile that owns the session.
func TestNowPlayingWaitCommand(t *testing.T) {
	reg := appmusic.NewNowPlayingRegistry()
	h := &musicHandler{nowPlaying: reg, commandWait: 2 * time.Second}
	owner := &domainuser.Claims{UserID: "u1", Username: "alice", ProfileID: "p1"}
	reg.Heartbeat("s1", "p1", "alice", "t1", "Kitchen", 0, false)

	for name, claims := range map[string]*domainuser.Claims{
		"other profile": {UserID: "u2", Username: "bob", ProfileID: "p2"},
		"no profile":    {UserID: "u1"},
	} {
		w := httptest.NewRecorder()
		h.nowPlayingWaitCommand(w, waitCommandReq("s1", claims))
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.nowPlayingWaitCommand(w, waitCommandReq("unknown", owner))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown session: %d, want 404", w.Code)
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.nowPlayingWaitCommand(rec, waitCommandReq("s1", owner))
		done <- rec
	}()
	select {
	case <-done:
		t.Fatal("the poll answered before any command was queued")
	case <-time.After(50 * time.Millisecond):
	}
	start := time.Now()
	reg.SetCommand("s1", "pause")
	rec := <-done
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("command took %v to come back", elapsed)
	}
	var resp map[string]string
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || resp["remoteCommand"] != "pause" {
		t.Fatalf("poll: %d %s", rec.Code, rec.Body.String())
	}

	// Nothing queued: the poll ends empty at its deadline so the player re-polls.
	h.commandWait = 30 * time.Millisecond
	w = httptest.NewRecorder()
	h.nowPlayingWaitCommand(w, waitCommandReq("s1", owner))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"remoteCommand":""`) {
		t.Errorf("empty poll: %d %s", w.Code, w.Body.String())
	}
}

func TestToSegmentDTOs(t *testing.T) {
	if got := toSegmentDTOs(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty input must encode as [], got %#v", got)
	}
	got := toSegmentDTOs([]domainlibrary.MediaSegment{{
		ID: "seg-1", MediaItemID: "mi-1", Type: "intro", StartMs: 1000, EndMs: 61000, Confidence: 0.87, Source: "audio",
	}})
	want := segmentDTO{ID: "seg-1", MediaItemID: "mi-1", Type: "intro", StartMs: 1000, EndMs: 61000, Confidence: 0.87, Source: "audio"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
