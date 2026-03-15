package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsyncplay "github.com/gravity-zero/evey/internal/app/syncplay"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	domainsyncplay "github.com/gravity-zero/evey/internal/domain/syncplay"
	"golang.org/x/net/websocket"
)

func TestSyncPlayIDOR_CreateRoomDenied(t *testing.T) {
	// Verifies that a user without library access can't create a syncplay room
	// for a media item they shouldn't access.
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "attacker", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["vid-1"] = &domainlibrary.MediaItem{ID: "vid-1", LibraryID: "lib-private"}

	item, _ := getter.GetMediaItem(ctx, "vid-1")
	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, denyAll())
	if ok {
		t.Error("syncplay room creation should be blocked when no library access")
	}
}

func TestSyncPlayIDOR_CreateRoomAllowed(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "user-1", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["vid-1"] = &domainlibrary.MediaItem{ID: "vid-1", LibraryID: "lib-allowed"}

	item, _ := getter.GetMediaItem(ctx, "vid-1")
	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, allowAll())
	if !ok {
		t.Error("syncplay room creation should be allowed with library access")
	}
}

// fakeSyncPlayRepo is a minimal in-memory RoomRepository for HTTP handler tests.
type fakeSyncPlayRepo struct {
	rooms map[string]*domainsyncplay.Room
}

func newFakeSyncPlayRepo() *fakeSyncPlayRepo {
	return &fakeSyncPlayRepo{rooms: make(map[string]*domainsyncplay.Room)}
}

func (r *fakeSyncPlayRepo) Create(_ context.Context, room *domainsyncplay.Room) error {
	r.rooms[room.ID] = room
	return nil
}

func (r *fakeSyncPlayRepo) Get(_ context.Context, id string) (*domainsyncplay.Room, error) {
	room, ok := r.rooms[id]
	if !ok {
		return nil, http.ErrNoCookie // sentinel error
	}
	cp := *room
	return &cp, nil
}

func (r *fakeSyncPlayRepo) AddMember(_ context.Context, roomID string, member domainsyncplay.Member) error {
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	room.Members = append(room.Members, member)
	return nil
}

func (r *fakeSyncPlayRepo) RemoveMember(_ context.Context, roomID, userID string) error {
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	filtered := room.Members[:0]
	for _, m := range room.Members {
		if m.UserID != userID {
			filtered = append(filtered, m)
		}
	}
	room.Members = filtered
	return nil
}

func (r *fakeSyncPlayRepo) UpdateState(_ context.Context, roomID string, state domainsyncplay.PlaybackState) error {
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	room.State = state
	return nil
}

func (r *fakeSyncPlayRepo) Delete(_ context.Context, id string) error {
	delete(r.rooms, id)
	return nil
}

func newSyncPlayTestMux(t *testing.T) (*http.ServeMux, *appsyncplay.Hub, *fakeSyncPlayRepo) {
	t.Helper()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}

	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())
	return mux, hub, repo
}

func TestSyncPlayRoomCreate_NoAuth_401(t *testing.T) {
	t.Parallel()
	mux, _, _ := newSyncPlayTestMux(t)

	body := `{"mediaId":"media-1","name":"Movie Night"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayRoomCreate_InvalidToken_401(t *testing.T) {
	t.Parallel()
	mux, _, _ := newSyncPlayTestMux(t)

	body := `{"mediaId":"media-1","name":"Movie Night"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid.token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayRoomCreate_Success(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}

	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())

	claims := domainuser.Claims{UserID: "host-1", Username: "Alice", IsAdmin: false}
	token, err := tokenGen.GenerateAccessToken(claims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	body := `{"mediaId":"media-1","name":"Movie Night"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["id"] == "" || resp["id"] == nil {
		t.Error("response should contain room id")
	}
	if resp["mediaId"] != "media-1" {
		t.Errorf("mediaId = %v, want media-1", resp["mediaId"])
	}
}

func TestSyncPlayRoomGet_NotFound_404(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, newFakeMediaGetter(), allowAll())

	claims := domainuser.Claims{UserID: "user-1", Username: "Alice", IsAdmin: false}
	token, err := tokenGen.GenerateAccessToken(claims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/nonexistent-room", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSyncPlayRoomGet_NoAuth_401(t *testing.T) {
	t.Parallel()
	mux, _, _ := newSyncPlayTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/some-room-id", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayRoomGet_Forbidden_NonMember(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())

	// Create room as host-1
	hostClaims := domainuser.Claims{UserID: "host-1", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	createBody := `{"mediaId":"media-1","name":"Private Room"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+hostToken)
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("room creation failed: %d", createW.Code)
	}
	var createResp map[string]any
	_ = json.NewDecoder(createW.Body).Decode(&createResp)
	roomID := createResp["id"].(string)

	// Attacker tries to GET room without being a member
	attackerClaims := domainuser.Claims{UserID: "attacker", Username: "Evil", IsAdmin: false}
	attackerToken, _ := tokenGen.GenerateAccessToken(attackerClaims)
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/"+roomID, nil)
	getReq.Header.Set("Authorization", "Bearer "+attackerToken)
	getW := httptest.NewRecorder()
	mux.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", getW.Code)
	}
}

// WebSocket auth tests (TEST-NEW-3)
// The pre-upgrade HTTP middleware returns 401/403 before the WS handshake,
// so plain HTTP requests suffice to test auth rejection.

func TestSyncPlayWS_NoToken_401(t *testing.T) {
	t.Parallel()
	mux, _, _ := newSyncPlayTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/any-room/ws", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayWS_InvalidToken_401(t *testing.T) {
	t.Parallel()
	mux, _, _ := newSyncPlayTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/any-room/ws", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSyncPlayWS_NonMember_403(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())

	// Create a room as host-1.
	hostClaims := domainuser.Claims{UserID: "host-1", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/rooms",
		strings.NewReader(`{"mediaId":"media-1","name":"Room"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+hostToken)
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("room creation failed: %d %s", createW.Code, createW.Body.String())
	}
	var createResp map[string]any
	_ = json.NewDecoder(createW.Body).Decode(&createResp)
	roomID := createResp["id"].(string)

	// Attacker (not a member, not the host) tries to connect via WS.
	attackerClaims := domainuser.Claims{UserID: "attacker", Username: "Evil", IsAdmin: false}
	attackerToken, _ := tokenGen.GenerateAccessToken(attackerClaims)
	wsReq := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/"+roomID+"/ws", nil)
	wsReq.Header.Set("Authorization", "Bearer "+attackerToken)
	wsW := httptest.NewRecorder()
	mux.ServeHTTP(wsW, wsReq)

	if wsW.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", wsW.Code)
	}
}

func TestSyncPlayWS_ValidHostToken_NotRejectedByAuth(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())

	// Use a real test server so the websocket upgrader has a proper hijackable conn.
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Create room as host-1 via the test server.
	hostClaims := domainuser.Claims{UserID: "host-1", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	createReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/syncplay/rooms",
		strings.NewReader(`{"mediaId":"media-1","name":"Room"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+hostToken)
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("room creation failed: %d", createResp.StatusCode)
	}
	var createBody map[string]any
	_ = json.NewDecoder(createResp.Body).Decode(&createBody)
	roomID := createBody["id"].(string)

	// Attempt a real WebSocket connection as the host.
	// The pre-upgrade middleware should pass (host is a member), resulting in 101.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + roomID + "/ws"
	origin := srv.URL
	ws, resp, err := dialWS(wsURL, origin, hostToken)
	if err == nil {
		ws.Close()
	}
	// Either success (101) or a non-auth error is acceptable; 401/403 must not occur.
	if resp != nil {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Error("valid host token must not be rejected with 401")
		}
		if resp.StatusCode == http.StatusForbidden {
			t.Error("room host must not be rejected with 403")
		}
	}
}

// TestSyncPlayWS_StateOnJoin verifies that the server immediately sends the current
// room state as the first message when a member connects via WebSocket.
func TestSyncPlayWS_StateOnJoin(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	hostClaims := domainuser.Claims{UserID: "host-state", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	// Create room.
	createReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/syncplay/rooms",
		strings.NewReader(`{"mediaId":"media-1","name":"StateRoom"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+hostToken)
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("room creation failed: %d", createResp.StatusCode)
	}
	var createBody map[string]any
	if decErr := json.NewDecoder(createResp.Body).Decode(&createBody); decErr != nil {
		t.Fatalf("decode create response: %v", decErr)
	}
	roomID := createBody["id"].(string)

	// Connect via WebSocket as host.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + roomID + "/ws"
	ws, _, err := dialWS(wsURL, srv.URL, hostToken)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer ws.Close()

	// First message must be type="state".
	var raw string
	if err := websocket.Message.Receive(ws.Conn, &raw); err != nil {
		t.Fatalf("receive state msg: %v", err)
	}
	var cmd domainsyncplay.SyncCommand
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatalf("unmarshal state msg: %v", err)
	}
	if cmd.Type != "state" {
		t.Errorf("expected first message type=state, got %q", cmd.Type)
	}
}

// TestSyncPlayWS_PlayPauseSeek verifies play/pause/seek messages are broadcast.
func TestSyncPlayWS_PlayPauseSeek(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	hostClaims := domainuser.Claims{UserID: "host-play", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	guestClaims := domainuser.Claims{UserID: "guest-play", Username: "Guest", IsAdmin: false}
	guestToken, err := tokenGen.GenerateAccessToken(guestClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	// Create room as host.
	createReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/syncplay/rooms",
		strings.NewReader(`{"mediaId":"media-1","name":"PlayRoom"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+hostToken)
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	defer createResp.Body.Close()
	var createBody map[string]any
	_ = json.NewDecoder(createResp.Body).Decode(&createBody)
	roomID := createBody["id"].(string)

	// Join room as guest via HTTP join endpoint so guest is a member.
	joinReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/syncplay/rooms/"+roomID+"/join", nil)
	joinReq.Header.Set("Authorization", "Bearer "+guestToken)
	joinResp, err := http.DefaultClient.Do(joinReq)
	if err != nil {
		t.Fatalf("join room: %v", err)
	}
	joinResp.Body.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + roomID + "/ws"

	// Connect host.
	hostWS, _, err := dialWS(wsURL, srv.URL, hostToken)
	if err != nil {
		t.Fatalf("host ws dial: %v", err)
	}
	defer hostWS.Close()

	// Drain host's own state message.
	var stateRaw string
	_ = websocket.Message.Receive(hostWS.Conn, &stateRaw)

	// Connect guest.
	guestWS, _, err := dialWS(wsURL, srv.URL, guestToken)
	if err != nil {
		t.Fatalf("guest ws dial: %v", err)
	}
	defer guestWS.Close()

	// Drain guest's state message.
	_ = websocket.Message.Receive(guestWS.Conn, &stateRaw)

	// Host sends "play".
	playMsg := `{"type":"play","positionMs":1000}`
	if err := websocket.Message.Send(hostWS.Conn, playMsg); err != nil {
		t.Fatalf("send play: %v", err)
	}

	// Guest should receive the play broadcast (skip member_joined messages first).
	for {
		var raw string
		if err := websocket.Message.Receive(guestWS.Conn, &raw); err != nil {
			t.Fatalf("guest receive: %v", err)
		}
		var cmd domainsyncplay.SyncCommand
		_ = json.Unmarshal([]byte(raw), &cmd)
		if cmd.Type == "member_joined" {
			continue // skip, drain
		}
		if cmd.Type != "play" {
			t.Errorf("expected play broadcast, got %q", cmd.Type)
		}
		if cmd.PositionMs != 1000 {
			t.Errorf("expected positionMs=1000, got %d", cmd.PositionMs)
		}
		break
	}
}

// TestSyncPlayWS_SubprotocolToken verifies that the token passed as a
// Sec-WebSocket-Protocol subprotocol ("Bearer.<token>") is accepted.
func TestSyncPlayWS_SubprotocolToken_Auth(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll())

	hostClaims := domainuser.Claims{UserID: "host-sub", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	// Build a request where Authorization header is absent but token is in
	// Sec-WebSocket-Protocol (simulates browser WebSocket API limitation).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/no-such-room/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "Bearer."+hostToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Room does not exist → 404, but auth (401) must NOT occur.
	if w.Code == http.StatusUnauthorized {
		t.Error("subprotocol bearer token should not be rejected with 401")
	}
}

// dialWS opens a websocket connection and returns the underlying *websocket.Conn,
// the HTTP response (even on failure), and any error.
func dialWS(url, origin, token string) (*wsDialConn, *http.Response, error) {
	cfg, err := websocket.NewConfig(url, origin)
	if err != nil {
		return nil, nil, err
	}
	cfg.Header = http.Header{"Authorization": {"Bearer " + token}}
	ws, err := websocket.DialConfig(cfg)
	if err != nil {
		// Extract status from error string if available.
		return nil, nil, err
	}
	return &wsDialConn{ws}, &http.Response{StatusCode: http.StatusSwitchingProtocols}, nil
}

type wsDialConn struct{ *websocket.Conn }

func (c *wsDialConn) Close() { _ = c.Conn.Close() }
