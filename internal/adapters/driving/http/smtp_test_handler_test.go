package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appconfig "github.com/evey-project/evey/internal/app/config"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

type fakeSMTPTester struct {
	err  error
	sent []string
}

func (f *fakeSMTPTester) SendTest(_ context.Context, to string) error {
	f.sent = append(f.sent, to)
	return f.err
}

type fakeSMTPUserGetter struct{ email string }

func (f fakeSMTPUserGetter) GetByID(_ context.Context, id shared.UserID) (*domainuser.User, error) {
	return &domainuser.User{ID: id, Email: f.email}, nil
}

func newSMTPTestMux(t *testing.T, isAdmin bool, sender *fakeSMTPTester, email string) (*http.ServeMux, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, err := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: isAdmin})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	mux := http.NewServeMux()
	registerSMTPTestRoute(mux, sender, fakeSMTPUserGetter{email: email}, gen, ratelimit.New(1000))
	return mux, token
}

func postSMTPTest(mux *http.ServeMux, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/smtp/test", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSMTPTestRoute_AdminOnly(t *testing.T) {
	t.Parallel()
	sender := &fakeSMTPTester{}
	mux, token := newSMTPTestMux(t, false, sender, "u@test.local")
	if rec := postSMTPTest(mux, token, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin: want 403, got %d", rec.Code)
	}
	if rec := postSMTPTest(mux, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", rec.Code)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("nothing may be sent for a refused caller, sent %v", sender.sent)
	}
}

func TestSMTPTestRoute_SendsToTheAdminsOwnAddress(t *testing.T) {
	t.Parallel()
	sender := &fakeSMTPTester{}
	mux, token := newSMTPTestMux(t, true, sender, "admin@test.local")
	rec := postSMTPTest(mux, token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sender.sent) != 1 || sender.sent[0] != "admin@test.local" {
		t.Fatalf("sent = %v", sender.sent)
	}

	rec = postSMTPTest(mux, token, `{"to":"Other <other@test.local>"}`)
	if rec.Code != http.StatusOK || sender.sent[1] != "other@test.local" {
		t.Fatalf("explicit recipient: code %d, sent %v", rec.Code, sender.sent)
	}
}

func decodeSMTPErr(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestSMTPTestRoute_SurfacesTheSMTPError(t *testing.T) {
	t.Parallel()
	sender := &fakeSMTPTester{err: errors.New("smtp plain auth: 535 5.7.8 bad credentials")}
	mux, token := newSMTPTestMux(t, true, sender, "admin@test.local")
	rec := postSMTPTest(mux, token, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rec.Code)
	}
	body := decodeSMTPErr(t, rec)
	if body["code"] != "send_failed" || !strings.Contains(body["message"], "535 5.7.8") {
		t.Fatalf("the admin must see the server's reason, got %v", body)
	}
}

func TestSMTPTestRoute_NotConfiguredAndBadRecipient(t *testing.T) {
	t.Parallel()
	sender := &fakeSMTPTester{err: appconfig.ErrSMTPNotConfigured}
	mux, token := newSMTPTestMux(t, true, sender, "")
	if rec := postSMTPTest(mux, token, ""); rec.Code != http.StatusBadRequest || decodeSMTPErr(t, rec)["code"] != "no_recipient" {
		t.Fatalf("admin without e-mail: want 400 no_recipient, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := postSMTPTest(mux, token, `{"to":"not an address"}`); rec.Code != http.StatusBadRequest || decodeSMTPErr(t, rec)["code"] != "invalid_recipient" {
		t.Fatalf("bad address: want 400 invalid_recipient, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := postSMTPTest(mux, token, `{"to":"a@test.local\r\nBcc: x@evil"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("header injection attempt: want 400, got %d", rec.Code)
	}
	rec := postSMTPTest(mux, token, `{"to":"a@test.local"}`)
	if rec.Code != http.StatusConflict || decodeSMTPErr(t, rec)["code"] != "not_configured" {
		t.Fatalf("no SMTP: want 409 not_configured, got %d %s", rec.Code, rec.Body.String())
	}
}
