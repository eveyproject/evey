package user

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAuthURL_SendsS256ChallengeNeverTheVerifier(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	const verifier = "a-verifier-that-must-stay-in-the-browser-0123"
	raw, err := svc.AuthURL(context.Background(), "n1", verifier)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, verifier) {
		t.Fatal("verifier in the authorization URL")
	}
	q, _ := url.Parse(raw)
	if q.Query().Get("code_challenge") != pkceChallenge(verifier) || q.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("challenge params: %v", q.Query())
	}
	state := q.Query().Get("state")
	if n, err := svc.ValidateBoundState(state, verifier); err != nil || n != "n1" {
		t.Fatalf("own verifier: %q %v", n, err)
	}
	if _, err := svc.ValidateBoundState(state, "another-login-verifier"); !errors.Is(err, ErrStateUnbound) {
		t.Fatalf("another verifier: %v", err)
	}
	if _, err := svc.ValidateBoundState(state, ""); !errors.Is(err, ErrStateUnbound) {
		t.Fatalf("no verifier: %v", err)
	}
	// Routing an IdP error needs no cookie.
	if n, err := svc.ValidateState(state); err != nil || n != "n1" {
		t.Fatalf("ValidateState: %q %v", n, err)
	}
	if _, err := svc.AuthURL(context.Background(), "n1", ""); err == nil {
		t.Fatal("AuthURL without a verifier")
	}
}

func TestValidateBoundState_StateWithoutChallengeIsUnbound(t *testing.T) {
	oidcSrv := newTestOIDCServer(t)
	defer oidcSrv.close()
	svc := buildOIDCService(t, oidcSrv)

	payload := strconv.FormatInt(time.Now().Unix(), 10) + "|n1"
	if _, err := svc.ValidateBoundState(payload+"."+svc.signState(payload), "v"); !errors.Is(err, ErrStateUnbound) {
		t.Fatalf("state without challenge: %v", err)
	}
}

func TestHandleCallback_ExchangesWithTheVerifier(t *testing.T) {
	sub := "sub-pkce"
	svc, oidcSrv := setupCallbackSvc(t, map[string]any{"sub": sub, "email": "p@example.com", "preferred_username": "p", "nonce": "n1"},
		func(s *OIDCService) { s.cur.cfg.AllowSignup = true })
	defer oidcSrv.close()
	var got string
	inner := oidcSrv.srv.Config.Handler
	oidcSrv.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			got = r.PostForm.Get("code_verifier")
		}
		inner.ServeHTTP(w, r)
	})

	if _, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "n1", ""); err == nil {
		t.Fatal("callback accepted without a verifier")
	}
	if _, _, _, err := svc.HandleCallback(context.Background(), "auth-code", "n1", "the-verifier"); err != nil {
		t.Fatal(err)
	}
	if got != "the-verifier" {
		t.Fatalf("token request code_verifier = %q", got)
	}
}
