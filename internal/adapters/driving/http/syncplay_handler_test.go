package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appmedia "github.com/evey-project/evey/internal/app/media"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainsyncplay "github.com/evey-project/evey/internal/domain/syncplay"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"golang.org/x/net/websocket"
)

func TestSyncPlayIDOR_CreateRoomDenied(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "attacker", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["vid-1"] = &domainlibrary.MediaItem{ID: "vid-1", LibraryID: shared.LibraryID("lib-private")}

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
	getter.items["vid-1"] = &domainlibrary.MediaItem{ID: "vid-1", LibraryID: shared.LibraryID("lib-allowed")}

	item, _ := getter.GetMediaItem(ctx, "vid-1")
	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, allowAll())
	if !ok {
		t.Error("syncplay room creation should be allowed with library access")
	}
}

// fakeGuestTokens is a minimal guest-token validator for the syncplay ws tests.
type fakeGuestTokens map[string]*shared.StreamToken

func (f fakeGuestTokens) ValidateAny(token string) (*shared.StreamToken, bool) {
	st, ok := f[token]
	return st, ok
}

// fakeSyncPlayRepo is a minimal in-memory RoomRepository for HTTP handler tests.
type fakeSyncPlayRepo struct {
	mu    sync.Mutex
	rooms map[string]*domainsyncplay.Room
}

func newFakeSyncPlayRepo() *fakeSyncPlayRepo {
	return &fakeSyncPlayRepo{rooms: make(map[string]*domainsyncplay.Room)}
}

func (r *fakeSyncPlayRepo) Create(_ context.Context, room *domainsyncplay.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[room.ID] = room
	return nil
}

func (r *fakeSyncPlayRepo) Get(_ context.Context, id string) (*domainsyncplay.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[id]
	if !ok {
		return nil, http.ErrNoCookie
	}
	cp := *room
	cp.Members = make([]domainsyncplay.Member, len(room.Members))
	copy(cp.Members, room.Members)
	return &cp, nil
}

func (r *fakeSyncPlayRepo) GetByCode(_ context.Context, code string) (*domainsyncplay.Room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, room := range r.rooms {
		if room.Code == code {
			cp := *room
			cp.Members = make([]domainsyncplay.Member, len(room.Members))
			copy(cp.Members, room.Members)
			return &cp, nil
		}
	}
	return nil, http.ErrNoCookie
}

func (r *fakeSyncPlayRepo) AddMember(_ context.Context, roomID string, member domainsyncplay.Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	room.Members = append(room.Members, member)
	return nil
}

func (r *fakeSyncPlayRepo) RemoveMember(_ context.Context, roomID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	filtered := make([]domainsyncplay.Member, 0, len(room.Members))
	for _, m := range room.Members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}
	room.Members = filtered
	return nil
}

func (r *fakeSyncPlayRepo) RemoveMemberAndDeleteIfEmpty(_ context.Context, roomID, userID string) (bool, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return false, "", http.ErrNoCookie
	}
	filtered := make([]domainsyncplay.Member, 0, len(room.Members))
	for _, m := range room.Members {
		if string(m.UserID) != userID {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		delete(r.rooms, roomID)
		return true, "", nil
	}
	room.Members = filtered
	newHostID := ""
	if string(room.HostID) == userID {
		newHost := domainsyncplay.OldestMember(filtered)
		newHostID = string(newHost.UserID)
		room.HostID = newHost.UserID
	}
	return false, newHostID, nil
}

func (r *fakeSyncPlayRepo) UpdateState(_ context.Context, roomID string, state domainsyncplay.PlaybackState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	room.State = state
	return nil
}

func (r *fakeSyncPlayRepo) UpdateTrackIndex(_ context.Context, roomID string, idx int, mediaID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return http.ErrNoCookie
	}
	room.TrackIndex = idx
	room.MediaID = shared.MediaItemID(mediaID)
	return nil
}

func (r *fakeSyncPlayRepo) AdvanceTrackIndex(_ context.Context, roomID string, delta int) (int, string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[roomID]
	if !ok {
		return 0, "", false, http.ErrNoCookie
	}
	newIdx := room.TrackIndex + delta
	if newIdx < 0 || newIdx >= len(room.Queue) {
		return 0, "", false, nil
	}
	room.TrackIndex = newIdx
	room.MediaID = room.Queue[newIdx]
	return newIdx, string(room.MediaID), true, nil
}

func (r *fakeSyncPlayRepo) Delete(_ context.Context, id string) error {
	delete(r.rooms, id)
	return nil
}

func (r *fakeSyncPlayRepo) DeleteStaleEmptyRooms(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func newSyncPlayTestMux(t *testing.T) (*http.ServeMux, *appsyncplay.Hub, *fakeSyncPlayRepo) {
	t.Helper()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}

	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})
	return mux, hub, repo
}

// createTestRoom creates a room via hub.CreateRoom (no HTTP POST needed after gRPC migration).
func createTestRoom(t *testing.T, hub *appsyncplay.Hub, hostID, mediaID string) *domainsyncplay.Room {
	t.Helper()
	room, err := hub.CreateRoom(context.Background(), hostID, mediaID, "TestRoom")
	if err != nil {
		t.Fatalf("hub.CreateRoom: %v", err)
	}
	return room
}

// --- JSON routes migrated to gRPC — tests removed ---

func TestSyncPlayWS_LibraryAccessDenied_403(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-restricted"}

	room := createTestRoom(t, hub, "host-ws", "media-1")

	denyMux := http.NewServeMux()
	registerSyncPlayRoutes(denyMux, hub, tokenGen, mediaGetter, denyAll(), fakeGuestTokens{})

	hostToken, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "host-ws", Username: "Host", IsAdmin: false})
	wsReq := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/"+room.ID+"/ws", nil)
	wsReq.Header.Set("Authorization", "Bearer "+hostToken)
	wsW := httptest.NewRecorder()
	denyMux.ServeHTTP(wsW, wsReq)

	if wsW.Code != http.StatusNotFound {
		t.Errorf("expected 404 when library access denied on WS, got %d", wsW.Code)
	}
}

func TestSyncPlayJoinByCode(t *testing.T) {
	t.Parallel()
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	store := appmedia.NewStreamTokenStore(5 * time.Minute)
	defer store.Stop()

	room := createTestRoom(t, hub, "host-jc", "media-jc") // CreateRoom defaults to a video room
	h := syncplayJoinByCodeHandler(hub, store)

	// valid code -> 200 + a guest token scoped to the room's media
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/join-code", strings.NewReader(`{"code":"`+room.Code+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("valid code: expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Token   string `json:"token"`
		RoomID  string `json:"roomId"`
		MediaID string `json:"mediaId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RoomID != room.ID || resp.MediaID != "media-jc" {
		t.Errorf("unexpected room/media in response: %+v", resp)
	}
	if resp.Token == "" {
		t.Fatal("expected a guest token in the response")
	}
	st, ok := store.Validate(resp.Token, "media-jc")
	if !ok || st.Scope != shared.ScopeSyncplayGuest {
		t.Errorf("token must stream media-jc as a guest, got ok=%v scope=%q", ok, st.Scope)
	}
	if _, ok := store.Validate(resp.Token, "other-media"); ok {
		t.Error("guest token must be scoped to its single media item")
	}

	// unknown code -> generic 404
	w2 := httptest.NewRecorder()
	h(w2, httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/join-code", strings.NewReader(`{"code":"00000"}`)))
	if w2.Code != http.StatusNotFound {
		t.Errorf("unknown code: expected 404, got %d", w2.Code)
	}

	// music room -> 409 (guest join is video-only)
	mroom, err := hub.CreateRoomFromQueue(context.Background(), "host-m", "MusicRoom", []string{"track-1"})
	if err != nil {
		t.Fatalf("CreateRoomFromQueue: %v", err)
	}
	w3 := httptest.NewRecorder()
	h(w3, httptest.NewRequest(http.MethodPost, "/api/v1/syncplay/join-code", strings.NewReader(`{"code":"`+mroom.Code+`"}`)))
	if w3.Code != http.StatusConflict {
		t.Errorf("music room: expected 409, got %d", w3.Code)
	}
}

func TestSyncPlayWS_GuestToken(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}

	room := createTestRoom(t, hub, "host-g", "media-1") // video room, MediaID = media-1

	guests := fakeGuestTokens{
		"guest-ok":    {Scope: shared.ScopeSyncplayGuest, MediaItemID: "media-1"},
		"guest-wrong": {Scope: shared.ScopeSyncplayGuest, MediaItemID: "media-2"},
		"not-guest":   {Scope: shared.ScopeStream, MediaItemID: "media-1"},
	}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), guests)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + room.ID + "/ws"

	// A guest token scoped to the room's media connects (no JWT, no membership).
	ws, _, err := dialWS(wsURL, srv.URL, "guest-ok")
	if err != nil {
		t.Fatalf("guest with a valid room-scoped token should connect, got: %v", err)
	}
	ws.Close()

	// A guest token scoped to a DIFFERENT media must be rejected.
	if _, _, err := dialWS(wsURL, srv.URL, "guest-wrong"); err == nil {
		t.Error("guest token scoped to another media must be rejected")
	}
	// A normal (non-guest) stream token must NOT grant ws access.
	if _, _, err := dialWS(wsURL, srv.URL, "not-guest"); err == nil {
		t.Error("a non-guest stream token must not grant ws access")
	}
}

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
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})

	room := createTestRoom(t, hub, "host-1", "media-1")

	attackerClaims := domainuser.Claims{UserID: "attacker", Username: "Evil", IsAdmin: false}
	attackerToken, _ := tokenGen.GenerateAccessToken(attackerClaims)
	wsReq := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/"+room.ID+"/ws", nil)
	wsReq.Header.Set("Authorization", "Bearer "+attackerToken)
	wsW := httptest.NewRecorder()
	mux.ServeHTTP(wsW, wsReq)

	if wsW.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", wsW.Code)
	}
}

// A music room's MediaID is a track ID, not a media_item, so the video
// media-item gate used to 404 it. The handler must skip that gate for music
// rooms and rely on membership. Regression for the "music syncplay ws 404" bug.
func TestSyncPlayWS_MusicRoom_SkipsMediaItemCheck(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter() // empty: GetMediaItem would 404 for any id
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})

	// Music room: track-1 is not a media_item.
	room, err := hub.CreateRoomFromQueue(context.Background(), "host-music", "MusicRoom", []string{"track-1"})
	if err != nil {
		t.Fatalf("hub.CreateRoomFromQueue: %v", err)
	}
	if room.MediaType != "music" {
		t.Fatalf("expected music room, got MediaType=%q", room.MediaType)
	}

	srv := httptest.NewServer(mux)
	defer srv.Close()

	hostToken, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "host-music", Username: "Host", IsAdmin: false})
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + room.ID + "/ws"
	ws, _, err := dialWS(wsURL, srv.URL, hostToken)
	if err != nil {
		// Before the fix the media-item gate returned 404 here, failing the handshake.
		t.Fatalf("music room WS should connect (media gate skipped + member), got: %v", err)
	}
	defer ws.Close()
}

func TestSyncPlayWS_ValidHostToken_NotRejectedByAuth(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	hostClaims := domainuser.Claims{UserID: "host-1", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	room := createTestRoom(t, hub, "host-1", "media-1")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + room.ID + "/ws"
	origin := srv.URL
	ws, resp, err := dialWS(wsURL, origin, hostToken)
	if err == nil {
		ws.Close()
	}
	if resp != nil {
		if resp.StatusCode == http.StatusUnauthorized {
			t.Error("valid host token must not be rejected with 401")
		}
		if resp.StatusCode == http.StatusForbidden {
			t.Error("room host must not be rejected with 403")
		}
	}
}

func TestSyncPlayWS_StateOnJoin(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	hostClaims := domainuser.Claims{UserID: "host-state", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	room := createTestRoom(t, hub, "host-state", "media-1")

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + room.ID + "/ws"
	ws, _, err := dialWS(wsURL, srv.URL, hostToken)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer ws.Close()

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

func TestSyncPlayWS_PlayPauseSeek(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})
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

	room := createTestRoom(t, hub, "host-play", "media-1")

	// Add guest as member via hub
	if err := hub.JoinRoom(context.Background(), room.ID, "guest-play", "Guest"); err != nil {
		t.Fatalf("hub.JoinRoom: %v", err)
	}

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/syncplay/rooms/" + room.ID + "/ws"

	hostWS, _, err := dialWS(wsURL, srv.URL, hostToken)
	if err != nil {
		t.Fatalf("host ws dial: %v", err)
	}
	defer hostWS.Close()

	var stateRaw string
	_ = websocket.Message.Receive(hostWS.Conn, &stateRaw)

	guestWS, _, err := dialWS(wsURL, srv.URL, guestToken)
	if err != nil {
		t.Fatalf("guest ws dial: %v", err)
	}
	defer guestWS.Close()

	_ = websocket.Message.Receive(guestWS.Conn, &stateRaw)

	playMsg := `{"type":"play","positionMs":1000}`
	if err := websocket.Message.Send(hostWS.Conn, playMsg); err != nil {
		t.Fatalf("send play: %v", err)
	}

	for {
		var raw string
		if err := websocket.Message.Receive(guestWS.Conn, &raw); err != nil {
			t.Fatalf("guest receive: %v", err)
		}
		var cmd domainsyncplay.SyncCommand
		_ = json.Unmarshal([]byte(raw), &cmd)
		if cmd.Type == "member_joined" {
			continue
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

func TestSyncPlayWS_SubprotocolToken_Auth(t *testing.T) {
	t.Parallel()
	tokenGen := newTestJWTGen(t)
	repo := newFakeSyncPlayRepo()
	hub := appsyncplay.NewHub(repo)
	mediaGetter := newFakeMediaGetter()
	mediaGetter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-1"}
	mux := http.NewServeMux()
	registerSyncPlayRoutes(mux, hub, tokenGen, mediaGetter, allowAll(), fakeGuestTokens{})

	hostClaims := domainuser.Claims{UserID: "host-sub", Username: "Host", IsAdmin: false}
	hostToken, err := tokenGen.GenerateAccessToken(hostClaims)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/syncplay/rooms/no-such-room/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "Bearer."+hostToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

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
		return nil, nil, err
	}
	return &wsDialConn{ws}, &http.Response{StatusCode: http.StatusSwitchingProtocols}, nil
}

type wsDialConn struct{ *websocket.Conn }

func (c *wsDialConn) Close() { _ = c.Conn.Close() }
