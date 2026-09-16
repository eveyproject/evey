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

	"github.com/evey-project/evey/internal/app/presence"
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
		"posSec": 3066.5,
		"state": "rs=4 buf=320.0-347.2 drop=12/2300 vis=visible"
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
		// The snapshot is what tells a full buffer from an empty one.
		"state": "rs=4 buf=320.0-347.2 drop=12/2300 vis=visible",
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

// --- cast presence + session touch (TV-driven now-playing) ----------------

type fakeCastPresence struct{ entries []presence.Entry }

func (f *fakeCastPresence) Touch(e presence.Entry) { f.entries = append(f.entries, e) }

type sessionTouch struct {
	profileID, mediaID string
	posMs              int64
}

func postCastBeaconWithPresence(t *testing.T, tr CastPresenceToucher, touch CastSessionTouch, eventType, body string, castToken bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(body))
	req.SetPathValue("id", "media-1")
	req.RemoteAddr = "192.168.1.42:51000"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Fuchsia) CrKey/1.56")
	if castToken {
		ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
			UserID: "usr-cast", ProfileID: "prof-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
		})
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	playerEventHandler(logger,
		WithPlayerEventCastPresence(tr, func(context.Context, string) string { return "gravity" }),
		WithPlayerEventCastSession(touch),
	)(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

// A TV liveness beacon must (1) surface the Chromecast as an online DEVICE of
// the casting user and (2) refresh the play session's position/updated_at -
// the phone that started the cast is frozen and its own signals are gone.
func TestPlayerEvent_CastBeaconTouchesPresenceAndSession(t *testing.T) {
	tr := &fakeCastPresence{}
	var touches []sessionTouch
	touch := func(_ context.Context, b CastBeacon) {
		touches = append(touches, sessionTouch{b.ProfileID, b.MediaItemID, b.PositionMs})
	}
	postCastBeaconWithPresence(t, tr, touch, "cast_receiver_alive", `{"type":"cast_receiver_alive","posSec":214.2}`, true)

	if len(tr.entries) != 1 {
		t.Fatalf("presence entries = %d, want 1", len(tr.entries))
	}
	e := tr.entries[0]
	if e.UserID != "usr-cast" || e.Username != "gravity" || e.ProfileID != "prof-cast" {
		t.Fatalf("presence identity = %+v", e)
	}
	if e.UserAgent != "EveyCast (Chromecast)" {
		t.Fatalf("presence UA = %q, want the EveyCast product UA", e.UserAgent)
	}
	if e.ClientIP != "192.168.1.42" {
		t.Fatalf("presence IP = %q", e.ClientIP)
	}
	if len(touches) != 1 || touches[0] != (sessionTouch{"prof-cast", "media-1", 214200}) {
		t.Fatalf("session touches = %+v", touches)
	}
}

// End-of-life beacons must not refresh presence or the session - a dying
// receiver is not an online device - and must END the session outright.
//
// They used to do nothing at all, on the reasoning that ageing out was the
// point. Ageing out is precisely what the alive beat prevented (it beats on a
// bare setInterval, through stalls, on a TV that is never really switched off),
// so the one moment the television announced its own death was the one moment
// the server threw away.
func TestPlayerEvent_CastEndBeaconsEndTheSession(t *testing.T) {
	for _, ev := range []string{"cast_receiver_shutdown", "cast_receiver_finished"} {
		t.Run(ev, func(t *testing.T) {
			tr := &fakeCastPresence{}
			var touches []sessionTouch
			var ended []sessionTouch
			touch := func(_ context.Context, b CastBeacon) { touches = append(touches, sessionTouch{b.ProfileID, b.MediaItemID, b.PositionMs}) }
			end := func(_ context.Context, p, m string) { ended = append(ended, sessionTouch{p, m, 0}) }

			req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
				strings.NewReader(`{"type":"`+ev+`"}`))
			req.SetPathValue("id", "media-1")
			req.RemoteAddr = "192.168.1.42:51000"
			ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
				UserID: "usr-cast", ProfileID: "prof-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
			})
			rec := httptest.NewRecorder()
			logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
			playerEventHandler(logger,
				WithPlayerEventCastPresence(tr, func(context.Context, string) string { return "gravity" }),
				WithPlayerEventCastSession(touch),
				WithPlayerEventCastSessionEnd(end),
			)(rec, req.WithContext(ctx))

			if len(ended) != 1 || ended[0] != (sessionTouch{"prof-cast", "media-1", 0}) {
				t.Fatalf("session ends = %+v, want one end for (prof-cast, media-1)", ended)
			}
			if len(tr.entries) != 0 || len(touches) != 0 {
				t.Fatalf("a dying receiver must not be refreshed (presence=%d touches=%d)", len(tr.entries), len(touches))
			}
		})
	}
}

// The end beacon carries no JWT bypass: only a cast stream token may end a
// session this way, or any authenticated client could end someone else's.
func TestPlayerEvent_CastEndBeaconIgnoresNonCastCallers(t *testing.T) {
	var ended []sessionTouch
	end := func(_ context.Context, p, m string) { ended = append(ended, sessionTouch{p, m, 0}) }
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
		strings.NewReader(`{"type":"cast_receiver_shutdown"}`))
	req.SetPathValue("id", "media-1")
	rec := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	playerEventHandler(logger, WithPlayerEventCastSessionEnd(end))(rec, req)
	if len(ended) != 0 {
		t.Fatalf("a non-cast caller must not end a session: %+v", ended)
	}
}

// Only a load may CREATE a session. An alive beat that could create one lets a
// stalled receiver rebuild the phantom row the reaper has just cleared - and on
// a television that never really switches off, that loop never ends.
func TestPlayerEvent_OnlyTheLoadBeaconMayCreateASession(t *testing.T) {
	for _, tc := range []struct {
		event string
		want  bool
	}{
		{"cast_receiver_loaded", true},
		{"cast_receiver_alive", false},
	} {
		t.Run(tc.event, func(t *testing.T) {
			var got []bool
			touch := func(_ context.Context, b CastBeacon) { got = append(got, b.AllowCreate) }
			req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
				strings.NewReader(`{"type":"`+tc.event+`","posSec":214.2}`))
			req.SetPathValue("id", "media-1")
			ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
				UserID: "usr-cast", ProfileID: "prof-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
			})
			rec := httptest.NewRecorder()
			logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
			playerEventHandler(logger, WithPlayerEventCastSession(touch))(rec, req.WithContext(ctx))
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("%s allowCreate = %+v, want [%v]", tc.event, got, tc.want)
			}
		})
	}
}

// The phone's own beacons ride a JWT: no cast presence, no session touch (the
// RPC ReportProgress path already covers the phone).
func TestPlayerEvent_CastPresenceIgnoresNonCastCallers(t *testing.T) {
	tr := &fakeCastPresence{}
	var touches []sessionTouch
	touch := func(_ context.Context, b CastBeacon) { touches = append(touches, sessionTouch{b.ProfileID, b.MediaItemID, b.PositionMs}) }
	postCastBeaconWithPresence(t, tr, touch, "cast_receiver_alive", `{"type":"cast_receiver_alive","posSec":10}`, false)
	if len(tr.entries) != 0 || len(touches) != 0 {
		t.Fatalf("JWT caller must not create cast presence (presence=%d touches=%d)", len(tr.entries), len(touches))
	}
}

// The alive beacon's RESPONSE carries the pending admin command: the receiver
// polls every 30s with it, so pause/stop/message reach the TV even when the
// phone that started the cast is frozen. Non-cast callers keep the bare 204.
func TestPlayerEvent_CastAliveBeaconCarriesPendingCommand(t *testing.T) {
	consume := func(_ context.Context, profileID, mediaID string) string {
		if profileID == "prof-cast" && mediaID == "media-1" {
			return "pause"
		}
		return ""
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
		strings.NewReader(`{"type":"cast_receiver_alive","posSec":10}`))
	req.SetPathValue("id", "media-1")
	req.RemoteAddr = "192.168.1.42:51000"
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID: "usr-cast", ProfileID: "prof-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
	})
	rec := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	playerEventHandler(logger, WithPlayerEventCastCommand(consume))(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when a command is pending", rec.Code)
	}
	var body struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Command != "pause" {
		t.Fatalf("body = %q err=%v, want {\"command\":\"pause\"}", rec.Body.String(), err)
	}

	// No pending command: plain 204, exactly the pre-feature shape.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
		strings.NewReader(`{"type":"cast_receiver_alive"}`))
	req2.SetPathValue("id", "media-1")
	playerEventHandler(logger, WithPlayerEventCastCommand(func(context.Context, string, string) string { return "" }))(rec2, req2.WithContext(ctx))
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 without a pending command", rec2.Code)
	}
}

// The relay's leg transition must reach the session row: the admin badge
// shows "transcodage" during the AAC leg and flips once the TV's wasm takes
// over (the server's ffmpeg dies at idle seconds later).
func TestPlayerEvent_CastAudioModeReachesTheSession(t *testing.T) {
	var got []string
	touch := func(_ context.Context, profileID, mediaID, mode string) {
		got = append(got, profileID+"|"+mediaID+"|"+mode)
	}
	post := func(body string, castToken bool) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(body))
		req.SetPathValue("id", "media-1")
		if castToken {
			ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
				UserID: "usr-cast", ProfileID: "prof-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
			})
			req = req.WithContext(ctx)
		}
		rec := httptest.NewRecorder()
		logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
		playerEventHandler(logger, WithPlayerEventCastAudioMode(touch))(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
		}
	}
	post(`{"type":"cast_receiver_audio_mode","details":"wasm"}`, true)
	post(`{"type":"cast_receiver_alive","details":"mode=wasm"}`, true) // not an audio_mode event
	post(`{"type":"cast_receiver_audio_mode","details":"aac"}`, false) // JWT caller: ignored
	if len(got) != 1 || got[0] != "prof-cast|media-1|wasm" {
		t.Fatalf("touches = %v, want exactly the cast audio_mode one", got)
	}
}

// The web player's own wasm beacon (user JWT, no stream token) reaches the
// session too: without it a browser decoding AC-3 itself stayed labelled as
// a server transcode on the admin panel.
func TestPlayerEvent_WebWasmAudioModeReachesTheSession(t *testing.T) {
	var got []string
	touch := func(_ context.Context, profileID, mediaID, mode string) {
		got = append(got, profileID+"|"+mediaID+"|"+mode)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event",
		strings.NewReader(`{"type":"wasm_audio_mode","details":"aac-latched"}`))
	req.SetPathValue("id", "media-1")
	ctx := context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "usr-1", ProfileID: "prof-web"})
	rec := httptest.NewRecorder()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	playerEventHandler(logger, WithPlayerEventCastAudioMode(touch))(rec, req.WithContext(ctx))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}
	if len(got) != 1 || got[0] != "prof-web|media-1|aac-latched" {
		t.Fatalf("touches = %v, want the web wasm_audio_mode one", got)
	}
}

// A sender reporting a receiver that stopped answering is not fatal to the
// client, yet it is exactly the line an operator searches for after "the
// phone kept spinning": it must land at WARN like a fatal error, not drown
// among the INFO beacons.
func TestPlayerEvent_CastSenderStallIsAWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	rec := postPlayerEvent(t, logger, `{
		"type": "cast_sender_stall",
		"details": "state=playing frozen=25s pos=3266",
		"fatal": false,
		"code": 0,
		"variant": 0,
		"segment": 0,
		"posSec": 3266
	}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("no parseable log line was written: %v (raw=%q)", err, buf.String())
	}
	if line["level"] != "WARN" || line["type"] != "cast_sender_stall" {
		t.Fatalf("level=%v type=%v, want WARN cast_sender_stall (line=%v)", line["level"], line["type"], line)
	}
}

// The paused flag has to survive the wire. It is the only thing separating a
// television somebody paused from one that is stuck, and if it were dropped in
// decoding, every pause would silently be read as a stall and reaped - with
// nothing anywhere to say why.
func TestPlayerEvent_CastBeaconCarriesThePausedFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"paused", `{"type":"cast_receiver_alive","posSec":214.2,"paused":true}`, true},
		{"playing", `{"type":"cast_receiver_alive","posSec":214.2,"paused":false}`, false},
		// A receiver older than the flag sends no such field. It must decode to
		// false so the playhead alone decides, which is what shipped before -
		// the published receiver bundle lags the source, so this is live.
		{"receiver too old to send it", `{"type":"cast_receiver_alive","posSec":214.2}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []bool
			touch := func(_ context.Context, b CastBeacon) { got = append(got, b.Paused) }
			req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/player-event", strings.NewReader(tc.body))
			req.SetPathValue("id", "media-1")
			ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
				UserID: "usr-cast", ProfileID: "prof-cast", MediaItemID: "media-1", Scope: "stream", Cast: true,
			})
			rec := httptest.NewRecorder()
			logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
			playerEventHandler(logger, WithPlayerEventCastSession(touch))(rec, req.WithContext(ctx))
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("paused = %+v, want [%v]", got, tc.want)
			}
		})
	}
}
