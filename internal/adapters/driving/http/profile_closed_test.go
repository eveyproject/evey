package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

type parisClock struct{ loc *time.Location }

func (c parisClock) Location() *time.Location { return c.loc }

// A Monday 8:00 in Paris: the default kids hours open at 18:00.
func closedKidsFixture(t *testing.T) (*http.ServeMux, func(claims domainuser.Claims) string, *appuser.ProfileSchedules) {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	gen, _, _ := genTokens(t)
	repo := userfakes.NewFakeProfileRepository()
	ctx := context.Background()
	sched := domainuser.DefaultKidsSchedule()
	_ = repo.Create(ctx, &domainuser.Profile{ID: "kid", UserID: "u1", Name: "Kid", IsKids: true, Schedule: &sched})
	_ = repo.Create(ctx, &domainuser.Profile{ID: "parent", UserID: "u1", Name: "Parent", Parental: &domainuser.ParentalSettings{PINHash: "hashed:1234"}})
	hasher := userfakes.NewFakePasswordHasher()
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, loc)
	hours := appuser.NewProfileSchedules(repo, hasher, parisClock{loc}).WithNow(func() time.Time { return now })

	mux := http.NewServeMux()
	parental := registerParentalRoutes(mux, repo, userfakes.NewFakeUserRepository(), hasher, gen)
	registerScheduleRoutes(mux, parental, hours)
	authMw := httpAuthMiddleware(gen)
	streamMw := httpStreamAuthMiddleware(gen)
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("GET /api/v1/feeds/home", authMw(ok))
	mux.HandleFunc("GET /api/v1/media/{id}/hls/seg.m4s", streamMw(ok))

	SetProfileAccessGate(hours)
	t.Cleanup(func() { SetProfileAccessGate(nil) })
	tok := func(c domainuser.Claims) string {
		s, err := gen.GenerateAccessToken(c)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	return mux, tok, hours
}

func do(mux *http.ServeMux, method, path, token, body string, ctx context.Context) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// Outside its hours a kids profile gets profile_closed on browsing AND on the
// bytes (a JWT or a stream token alike), keeps the routes it needs to reopen,
// and a parent's PIN opens it until the next opening. Adults are untouched.
func TestProfileClosed_EveryRouteRefusedUntilAParentPIN(t *testing.T) {
	mux, tok, _ := closedKidsFixture(t)
	kid := tok(domainuser.Claims{UserID: "u1", ProfileID: "kid"})
	parent := tok(domainuser.Claims{UserID: "u1", ProfileID: "parent"})

	w := do(mux, http.MethodGet, "/api/v1/feeds/home", kid, "", nil)
	if w.Code != http.StatusForbidden || w.Header().Get("X-Evey-Error") != "profile_closed" {
		t.Fatalf("closed kid browsing: %d %q, want 403 profile_closed", w.Code, w.Header().Get("X-Evey-Error"))
	}
	var body struct {
		Details []struct {
			Reason   string            `json:"reason"`
			Metadata map[string]string `json:"metadata"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Details) != 1 || body.Details[0].Reason != ProfileClosedReason {
		t.Fatalf("refusal body %s: want an ErrorInfo PROFILE_CLOSED", w.Body.String())
	}
	if body.Details[0].Metadata["profile_id"] != "kid" {
		t.Errorf("profile_id = %q, want the closed profile", body.Details[0].Metadata["profile_id"])
	}
	if body.Details[0].Metadata["next_open"] != "2026-10-05T16:00:00Z" {
		t.Errorf("next_open = %q, want 18:00 Paris", body.Details[0].Metadata["next_open"])
	}

	if w := do(mux, http.MethodGet, "/api/v1/media/m1/hls/seg.m4s", kid, "", nil); w.Code != http.StatusForbidden {
		t.Errorf("closed kid segment over JWT: %d, want 403", w.Code)
	}
	stCtx := context.WithValue(context.Background(), streamTokenCtxKey, &shared.StreamToken{UserID: "u1", ProfileID: "kid", MediaItemID: "m1"})
	if w := do(mux, http.MethodGet, "/api/v1/media/m1/hls/seg.m4s", "", "", stCtx); w.Code != http.StatusForbidden {
		t.Errorf("closed kid segment over a stream token: %d, want 403", w.Code)
	}
	if w := do(mux, http.MethodGet, "/api/v1/feeds/home", parent, "", nil); w.Code != http.StatusOK {
		t.Errorf("adult profile: %d, want 200", w.Code)
	}

	if w := do(mux, http.MethodGet, "/api/v1/profiles/kid/schedule", kid, "", nil); w.Code != http.StatusOK {
		t.Errorf("the closed kid reads its hours: %d, want 200", w.Code)
	}
	if w := do(mux, http.MethodPost, "/api/v1/profiles/kid/schedule/unlock", kid, `{"pin":"9999"}`, nil); w.Code != http.StatusForbidden {
		t.Errorf("wrong PIN: %d, want 403", w.Code)
	}
	if w := do(mux, http.MethodPost, "/api/v1/profiles/kid/schedule/unlock", kid, `{"pin":"1234"}`, nil); w.Code != http.StatusOK {
		t.Fatalf("parent PIN: %d %s, want 200", w.Code, w.Body.String())
	}
	if w := do(mux, http.MethodGet, "/api/v1/feeds/home", kid, "", nil); w.Code != http.StatusOK {
		t.Errorf("after the parent PIN: %d, want 200", w.Code)
	}
}

// The kids profile cannot edit its own hours; a parent can, and invalid
// hours are refused with a reason.
func TestScheduleUpdate_ParentOnlyAndValidated(t *testing.T) {
	mux, tok, _ := closedKidsFixture(t)
	kid := tok(domainuser.Claims{UserID: "u1", ProfileID: "kid"})
	parent := tok(domainuser.Claims{UserID: "u1", ProfileID: "parent"})
	allDay := `{"schedule":{"enabled":true,"mode":"daily","rules":[{"days":[0,1,2,3,4,5,6],"ranges":[{"start":0,"end":1440}]}]}}`

	if w := do(mux, http.MethodPut, "/api/v1/profiles/kid/schedule", kid, allDay, nil); w.Code != http.StatusForbidden {
		t.Errorf("kid editing its own hours: %d, want 403", w.Code)
	}
	if w := do(mux, http.MethodPut, "/api/v1/profiles/kid/schedule", parent, `{"schedule":{"enabled":true,"mode":"daily","rules":[{"days":[1],"ranges":[{"start":60,"end":60}]}]}}`, nil); w.Code != http.StatusBadRequest {
		t.Errorf("invalid hours: %d, want 400", w.Code)
	}
	w := do(mux, http.MethodPut, "/api/v1/profiles/kid/schedule", parent, allDay, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("parent saving: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"open":true`) {
		t.Errorf("saved all-day hours must read open: %s", w.Body.String())
	}
	if w := do(mux, http.MethodGet, "/api/v1/feeds/home", kid, "", nil); w.Code != http.StatusOK {
		t.Errorf("new hours apply at once: %d, want 200", w.Code)
	}
	if w := do(mux, http.MethodPut, "/api/v1/profiles/parent/schedule", parent, allDay, nil); w.Code != http.StatusBadRequest {
		t.Errorf("hours on an adult profile: %d, want 400", w.Code)
	}
}

type closesSoonGate struct{ at time.Time }

func (g closesSoonGate) Access(context.Context, string) (domainuser.ProfileAccess, error) {
	return domainuser.ProfileAccess{Open: true, ClosesAt: g.at}, nil
}

// A response still being written at closing time - a film read as one long
// range, a download - is cut then: writes fail and the request context ends,
// so a player that reports no progress cannot keep watching.
func TestProfileClosing_CutsAResponseStillStreamingAtClosingTime(t *testing.T) {
	gen, _, _ := genTokens(t)
	SetProfileAccessGate(closesSoonGate{at: time.Now().Add(150 * time.Millisecond)})
	t.Cleanup(func() { SetProfileAccessGate(nil) })

	written, ctxEnded := 0, false
	stream := func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1024)
		for range 20 {
			if _, err := w.Write(chunk); err != nil {
				break
			}
			written++
			select {
			case <-r.Context().Done():
				ctxEnded = true
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", httpStreamAuthMiddleware(gen)(stream))
	tok, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "kid"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	w := do(mux, http.MethodGet, "/api/v1/media/m1/stream", tok, "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("open at the start: %d, want 200", w.Code)
	}
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Errorf("the response ran %v, want it cut shortly after the 150 ms closing", elapsed)
	}
	if written >= 20 || !ctxEnded {
		t.Errorf("wrote %d of 20 chunks, context ended=%v: want the stream stopped by the closing", written, ctxEnded)
	}
}

// A client that stops reading leaves the server's write blocked in the
// kernel: no later Write happens to notice the closing, and the context does
// not unblock it. The network deadline does - even when the handler cleared
// it for a long stream, as the music and direct-play handlers do.
func TestProfileClosing_UnblocksAWriteStuckOnASlowClient(t *testing.T) {
	gen, _, _ := genTokens(t)
	SetProfileAccessGate(closesSoonGate{at: time.Now().Add(400 * time.Millisecond)})
	t.Cleanup(func() { SetProfileAccessGate(nil) })

	returned := make(chan time.Time, 1)
	stream := func(w http.ResponseWriter, _ *http.Request) {
		defer func() { returned <- time.Now() }()
		// What music_handler does before ServeContent: "no deadline".
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		chunk := make([]byte, 256<<10)
		for range 4096 { // 1 GiB, far more than any socket buffer holds
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/tracks/{id}/stream", httpStreamAuthMiddleware(gen)(stream))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tok, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "kid"})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(4096)
	}
	_, err = fmt.Fprintf(conn, "GET /api/v1/music/tracks/t1/stream HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer %s\r\n\r\n", tok)
	if err != nil {
		t.Fatal(err)
	}
	// The client reads nothing more: the server's writes fill the buffers
	// and block.
	start := time.Now()
	select {
	case at := <-returned:
		if took := at.Sub(start); took > 2*time.Second {
			t.Errorf("the stuck write ended after %v, want shortly after the 400 ms closing", took)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the write stayed blocked on the slow client past the closing time")
	}
}
