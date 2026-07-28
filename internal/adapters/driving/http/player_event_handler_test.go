package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	shared "github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// The 2026-07-15 incident: a viewer watched a rainbow-garbled screen for
// minutes, then an endless spinner - and the server logs carried NOT ONE line
// about it. Everything the player knew (decode errors, recovery attempts,
// element-level MediaError) stayed in the browser console. This beacon is how
// a client-side playback failure becomes a server-side, searchable log line.
func postPlayerEvent(t *testing.T, logger *slog.Logger, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(body))
	req.SetPathValue("id", "media-1")
	rec := httptest.NewRecorder()
	playerEventHandler(logger)(rec, req)
	return rec
}

func TestPlayerEvent_LogsAClientErrorWithItsContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	rec := postPlayerEvent(t, logger, `{
		"type": "hls_media_error",
		"details": "bufferAppendError",
		"fatal": true,
		"code": 3,
		"variant": 2,
		"segment": 1023,
		"posSec": 3066.5
	}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("no parseable log line was written: %v (raw=%q)", err, buf.String())
	}
	for field, want := range map[string]any{
		"media_id":  "media-1",
		"component": "player_client",
		"type":      "hls_media_error",
		"details":   "bufferAppendError",
		"fatal":     true,
		"level":     "WARN",
	} {
		if got := line[field]; got != want {
			t.Fatalf("log field %q = %v, want %v (line=%v)", field, got, want, line)
		}
	}
	if got := line["seg"]; got != float64(1023) {
		t.Fatalf("log field seg = %v, want 1023", line["seg"])
	}
}

// Garbage in must never crash or pollute: no type -> 400, an unparseable body
// -> 400, and free-text fields are clamped so a hostile client cannot pump
// megabytes into the log files through this door.
func TestPlayerEvent_RejectsAndClamps(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	if rec := postPlayerEvent(t, logger, `{"details":"no type"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing type: status = %d, want 400", rec.Code)
	}
	if rec := postPlayerEvent(t, logger, `{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: status = %d, want 400", rec.Code)
	}
	if rec := postPlayerEvent(t, logger, `{"type":"weird type; DROP TABLE"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("type outside [a-z0-9_]: status = %d, want 400", rec.Code)
	}
	if rec := postPlayerEvent(t, logger, `{"type":"x","details":"`+strings.Repeat("A", 5000)+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized body: status = %d, want 400", rec.Code)
	}

	buf.Reset()
	long := strings.Repeat("d", 500)
	if rec := postPlayerEvent(t, logger, `{"type":"hls_fatal","details":"`+long+`"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("long-but-in-budget details: status = %d, want 204", rec.Code)
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line: %v", err)
	}
	if details, _ := line["details"].(string); len(details) > 200 {
		t.Fatalf("details logged at %d chars, want clamped to 200", len(details))
	}
}

// The cast receiver has no user JWT - its stream token (?token=, purpose=cast)
// is its whole identity. The route accepts it via streamAuth, and the handler
// must pin the beacon to the token's OWN media: a leaked cast token must not
// let anyone spray beacons onto arbitrary media ids (2026-07-26: the receiver
// telemetry that diagnoses TV-side cast deaths rides on this).
func postPlayerEventWithStreamToken(t *testing.T, logger *slog.Logger, pathMediaID, tokenMediaID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+pathMediaID+"/player-event",
		strings.NewReader(`{"type":"cast_receiver_error","details":"code=905"}`))
	req.SetPathValue("id", pathMediaID)
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "usr-cast",
		MediaItemID: shared.MediaItemID(tokenMediaID),
		Scope:       "stream",
		Cast:        true,
	})
	rec := httptest.NewRecorder()
	playerEventHandler(logger)(rec, req.WithContext(ctx))
	return rec
}

func TestPlayerEvent_StreamTokenBoundToItsOwnMedia(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	if rec := postPlayerEventWithStreamToken(t, logger, "media-1", "media-1"); rec.Code != http.StatusNoContent {
		t.Fatalf("matching token: status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line: %v", err)
	}
	if line["media_id"] != "media-1" || line["type"] != "cast_receiver_error" {
		t.Fatalf("unexpected log line: %v", line)
	}

	buf.Reset()
	if rec := postPlayerEventWithStreamToken(t, logger, "media-OTHER", "media-1"); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-media token: status = %d, want 403", rec.Code)
	}
	if buf.Len() != 0 {
		t.Fatalf("cross-media beacon must not be logged, got %q", buf.String())
	}
}

// postPlayerEventAsClaims mirrors postPlayerEvent but injects JWT claims
// into the context, wiring the optional library/parental guard.
func postPlayerEventAsClaims(t *testing.T, logger *slog.Logger, claims *domainuser.Claims, opts ...PlayerEventOption) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(`{"type":"hls_fatal"}`))
	req.SetPathValue("id", "media-1")
	req = withHTTPClaims(req, claims)
	rec := httptest.NewRecorder()
	playerEventHandler(logger, opts...)(rec, req)
	return rec
}

// TestPlayerEvent_NoGuardWired_UnchangedBehavior proves the guard is a no-op
// until a caller wires WithPlayerEventLibraryGuard - the migration aid that
// keeps the beacon route working exactly as before server.go passes the new
// dependencies in.
func TestPlayerEvent_NoGuardWired_UnchangedBehavior(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	rec := postPlayerEventAsClaims(t, logger, &domainuser.Claims{UserID: "u1"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (no guard wired)", rec.Code)
	}
}

// TestPlayerEvent_LibraryAccessDenied_NotFound proves that once the guard is
// wired, a JWT holder without access to media-1's library can no longer push
// events onto it (previously: any valid JWT could target any media_id).
func TestPlayerEvent_LibraryAccessDenied_NotFound(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	media := newFakeMediaGetter()
	media.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-restricted"}
	guard := WithPlayerEventLibraryGuard(media, denyAll(), nil)

	rec := postPlayerEventAsClaims(t, logger, &domainuser.Claims{UserID: "attacker"}, guard)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (library access denied)", rec.Code)
	}
}

// TestPlayerEvent_LibraryAccessAllowed_NotBlocked is the anti-over-block
// guard: a caller with real access to the media's library must keep working.
func TestPlayerEvent_LibraryAccessAllowed_NotBlocked(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	media := newFakeMediaGetter()
	media.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	guard := WithPlayerEventLibraryGuard(media, allowAll(), allowAllParental{})

	rec := postPlayerEventAsClaims(t, logger, &domainuser.Claims{UserID: "u1"}, guard)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (library access allowed)", rec.Code)
	}
}

// TestPlayerEvent_ParentalBlocked_NotFound proves the parental gate applies
// once wired: library access alone isn't enough for a kids profile pointed at
// an adult title.
func TestPlayerEvent_ParentalBlocked_NotFound(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	media := newFakeMediaGetter()
	media.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	guard := WithPlayerEventLibraryGuard(media, allowAll(), alwaysBlockChecker{})

	rec := postPlayerEventAsClaims(t, logger, &domainuser.Claims{UserID: "kid", ProfileID: "kid-profile"}, guard)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (parental blocked)", rec.Code)
	}
}

// TestPlayerEvent_AdminBypassesParental mirrors checkParental's own admin
// bypass: an admin profile must not be blocked even by an always-block
// checker.
func TestPlayerEvent_AdminBypassesParental(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	media := newFakeMediaGetter()
	media.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	guard := WithPlayerEventLibraryGuard(media, allowAll(), alwaysBlockChecker{})

	rec := postPlayerEventAsClaims(t, logger, &domainuser.Claims{UserID: "admin", IsAdmin: true}, guard)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (admin bypasses parental)", rec.Code)
	}
}

// TestPlayerEvent_StreamTokenBypassesGuardEvenWhenWired proves the cast
// receiver's stream-token path keeps working unchanged even once the
// claims-based guard is wired: the token's own media scope is the
// authorization, not the (absent) JWT claims.
func TestPlayerEvent_StreamTokenBypassesGuardEvenWhenWired(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	media := newFakeMediaGetter() // empty: a claims-based lookup would 404
	guard := WithPlayerEventLibraryGuard(media, denyAll(), alwaysBlockChecker{})

	// Construct the handler WITH the guard wired, deny-all/block-all - a
	// matching stream token must still pass, unaffected by either.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(`{"type":"cast_receiver_error"}`))
	req.SetPathValue("id", "media-1")
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID: "usr-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
	})
	rec2 := httptest.NewRecorder()
	playerEventHandler(logger, guard)(rec2, req.WithContext(ctx))
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (stream token bypasses the claims guard)", rec2.Code)
	}
}

// The admin log search must file these under where=player: that is where an
// operator investigating a playback incident already looks.
func TestClassify_PlayerClientComponentIsPlayer(t *testing.T) {
	where, _ := classifyLog("player_client", "player-client: playback error reported", "")
	if where != wherePlayer {
		t.Fatalf("where = %q, want %q", where, wherePlayer)
	}
}

// --- co-sender trigger ---

// The co-sender protects a cast session from its own phone: when the mobile
// sender freezes, the box tears the receiver down as "last sender
// disconnected" (~3min40 measured in prod, 2026-07-28). Each receiver beacon
// arrives FROM the TV, so its source address IS the TV - the beacon both
// names the target and re-arms the attach every 30s.
type fakeCoSender struct {
	attached []string
	detached []string
}

func (f *fakeCoSender) Attach(addr string) { f.attached = append(f.attached, addr) }
func (f *fakeCoSender) Detach(addr string) { f.detached = append(f.detached, addr) }

func postReceiverBeacon(t *testing.T, cs CastCoSender, eventType, remoteAddr string, castToken bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
		strings.NewReader(`{"type":"`+eventType+`"}`))
	req.SetPathValue("id", "media-1")
	req.RemoteAddr = remoteAddr
	if castToken {
		ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
			UserID: "usr-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
		})
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	playerEventHandler(logger, WithPlayerEventCoSender(cs))(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPlayerEvent_CoSenderAttachOnAliveBeacons(t *testing.T) {
	cs := &fakeCoSender{}
	postReceiverBeacon(t, cs, "cast_receiver_loaded", "192.168.1.42:51000", true)
	postReceiverBeacon(t, cs, "cast_receiver_alive", "192.168.1.42:51001", true)
	if len(cs.attached) != 2 || cs.attached[0] != "192.168.1.42" {
		t.Fatalf("attached = %v, want two attaches for 192.168.1.42", cs.attached)
	}
	if len(cs.detached) != 0 {
		t.Fatalf("no detach expected, got %v", cs.detached)
	}
}

func TestPlayerEvent_CoSenderDetachOnSessionEnd(t *testing.T) {
	cs := &fakeCoSender{}
	postReceiverBeacon(t, cs, "cast_receiver_shutdown", "192.168.1.42:51000", true)
	postReceiverBeacon(t, cs, "cast_receiver_finished", "192.168.1.42:51001", true)
	if len(cs.detached) != 2 {
		t.Fatalf("detached = %v, want two detaches", cs.detached)
	}
	if len(cs.attached) != 0 {
		t.Fatalf("no attach expected, got %v", cs.attached)
	}
}

func TestPlayerEvent_CoSenderIgnoresNonCastCallers(t *testing.T) {
	// The phone's own beacons (JWT, no cast stream token) name the PHONE's
	// address, not the TV's: attaching to it would dial the wrong device.
	cs := &fakeCoSender{}
	postReceiverBeacon(t, cs, "cast_receiver_loaded", "192.168.1.10:40000", false)
	if len(cs.attached) != 0 {
		t.Fatalf("JWT caller must not trigger the co-sender, got %v", cs.attached)
	}
}

func TestPlayerEvent_CoSenderIgnoresPublicAddresses(t *testing.T) {
	// A remote user's TV posts beacons from a public IP: the server cannot
	// reach it on 8009, and this must never become a knob to make the server
	// dial arbitrary internet hosts.
	cs := &fakeCoSender{}
	postReceiverBeacon(t, cs, "cast_receiver_alive", "203.0.113.7:44000", true)
	if len(cs.attached) != 0 {
		t.Fatalf("public caller must not trigger the co-sender, got %v", cs.attached)
	}
}
