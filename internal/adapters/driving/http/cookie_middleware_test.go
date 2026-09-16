package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthCookieMiddleware_SetsCookie(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"accessToken":  "at-value",
			"refreshToken": "rt-value",
		})
	})
	h := authCookieMiddleware(inner, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.Value == "rt-value" {
			found = true
			if !c.HttpOnly {
				t.Error("cookie should be httpOnly")
			}
		}
	}
	if !found {
		t.Error("refresh cookie not set")
	}
}

func TestAuthCookieMiddleware_NoCookie_NonJSON(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := authCookieMiddleware(inner, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName {
			t.Error("should not set cookie for non-JSON response")
		}
	}
}

func TestAuthCookieMiddleware_NoCookie_ErrorStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad"}`))
	})
	h := authCookieMiddleware(inner, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName {
			t.Error("should not set cookie for error responses")
		}
	}
}

func TestInjectRefreshToken_FromCookie_Injected(t *testing.T) {
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-rt"})

	r2 := injectRefreshTokenFromCookie(req)
	var m map[string]string
	if err := json.NewDecoder(r2.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["refreshToken"] != "cookie-rt" {
		t.Errorf("expected cookie-rt, got %q", m["refreshToken"])
	}
}

func TestInjectRefreshToken_FromCookie_NoOverwrite(t *testing.T) {
	body := []byte(`{"refreshToken":"body-rt"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-rt"})

	r2 := injectRefreshTokenFromCookie(req)
	var m map[string]string
	if err := json.NewDecoder(r2.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m["refreshToken"] != "body-rt" {
		t.Errorf("body token should win, got %q", m["refreshToken"])
	}
}

func TestClearRefreshCookie_SetsNegativeMaxAge(t *testing.T) {
	w := httptest.NewRecorder()
	clearRefreshCookie(w, false)
	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Error("cookie should be cleared with MaxAge -1")
	}
}

// The refresh cookie is minted from wherever the login body carries the
// token: top-level for login/refresh, nested under auth for QuickConnect;
// an empty or non-string value is no token at all.
func TestExtractRefreshToken(t *testing.T) {
	parse := func(s string) map[string]json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, tc := range []struct {
		body, wantTok, wantWhere string
	}{
		{`{"refreshToken":"rt-1","accessToken":"a"}`, "rt-1", "refreshToken"},
		{`{"auth":{"refreshToken":"rt-2"}}`, "rt-2", "auth"},
		{`{"refreshToken":"rt-top","auth":{"refreshToken":"rt-nested"}}`, "rt-top", "refreshToken"},
		{`{"refreshToken":"","auth":{"refreshToken":"rt-nested"}}`, "rt-nested", "auth"},
		{`{"refreshToken":42}`, "", ""},
		{`{"auth":"not-an-object"}`, "", ""},
		{`{"accessToken":"a"}`, "", ""},
	} {
		tok, where := extractRefreshToken(parse(tc.body))
		if tok != tc.wantTok || where != tc.wantWhere {
			t.Errorf("%s: got (%q,%q) want (%q,%q)", tc.body, tok, where, tc.wantTok, tc.wantWhere)
		}
	}
}
