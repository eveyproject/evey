package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"strings"
)

// extractRefreshToken finds refreshToken in the JSON body.
// It checks top-level "refreshToken" first (login/refresh responses),
// then "auth.refreshToken" (CheckQuickConnect response).
// Returns the token string and the key to delete from body.
func extractRefreshToken(body map[string]json.RawMessage) (string, string) {
	// Top-level: {"refreshToken": "..."}
	if rt, ok := body["refreshToken"]; ok {
		var s string
		if json.Unmarshal(rt, &s) == nil && s != "" {
			return s, "refreshToken"
		}
	}
	// Nested: {"auth": {"refreshToken": "...", "accessToken": "..."}}
	if authRaw, ok := body["auth"]; ok {
		var auth map[string]json.RawMessage
		if json.Unmarshal(authRaw, &auth) == nil {
			if rt, ok := auth["refreshToken"]; ok {
				var s string
				if json.Unmarshal(rt, &s) == nil && s != "" {
					// Strip refreshToken from nested auth, keep accessToken
					delete(auth, "refreshToken")
					if newAuth, err := json.Marshal(auth); err == nil {
						body["auth"] = newAuth
					}
					return s, "" // don't delete "auth" key, just modified it in-place
				}
			}
		}
	}
	return "", ""
}

// authCookieMiddleware wraps a handler: after the inner handler responds with an AuthResponse,
// it extracts refreshToken from the JSON body, sets an httpOnly cookie, and strips it from the body.
func authCookieMiddleware(next http.Handler, httpsOnly bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &authResponseRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		ct := rec.header.Get("Content-Type")
		if rec.code < 200 || rec.code >= 300 || !containsJSON(ct) {
			rec.flush(w)
			return
		}

		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.buf.Bytes(), &body); err != nil {
			rec.flush(w)
			return
		}

		rtStr, key := extractRefreshToken(body)
		if rtStr == "" {
			rec.flush(w)
			return
		}

		delete(body, key)
		http.SetCookie(w, &http.Cookie{
			Name:     refreshCookieName,
			Value:    rtStr,
			Path:     "/api/v1/auth/",
			HttpOnly: true,
			Secure:   httpsOnly,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   refreshCookieMaxAge,
		})

		newBody, err := json.Marshal(body)
		if err != nil {
			rec.flush(w)
			return
		}
		maps.Copy(w.Header(), rec.header)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.code)
		if _, err := w.Write(newBody); err != nil {
			return
		}
	})
}

func containsJSON(ct string) bool {
	return strings.Contains(ct, "application/json")
}

// injectRefreshTokenFromCookie reads the evey_refresh cookie and injects its value
// into the request body if refreshToken is not already present.
func injectRefreshTokenFromCookie(r *http.Request) *http.Request {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		return r
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return r
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var m map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(bodyBytes, &m); unmarshalErr != nil {
		m = make(map[string]json.RawMessage)
	}
	if _, exists := m["refreshToken"]; !exists {
		quoted, jsonErr := json.Marshal(cookie.Value)
		if jsonErr != nil {
			return r
		}
		m["refreshToken"] = quoted
	}
	merged, err := json.Marshal(m)
	if err != nil {
		return r
	}
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(merged))
	r2.ContentLength = int64(len(merged))
	return r2
}

// clearRefreshCookie deletes the httpOnly refresh cookie.
func clearRefreshCookie(w http.ResponseWriter, httpsOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth/",
		HttpOnly: true,
		Secure:   httpsOnly,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// clearOIDCIDTokenCookie deletes the OIDC id_token cookie used for RP-Initiated Logout.
func clearOIDCIDTokenCookie(w http.ResponseWriter, httpsOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcIDTokenCookieName,
		Value:    "",
		Path:     "/api/v1/auth/",
		HttpOnly: true,
		Secure:   httpsOnly,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type authResponseRecorder struct {
	http.ResponseWriter
	code   int
	buf    bytes.Buffer
	header http.Header
}

func (r *authResponseRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r *authResponseRecorder) WriteHeader(code int) { r.code = code }

func (r *authResponseRecorder) Write(b []byte) (int, error) { return r.buf.Write(b) }

func (r *authResponseRecorder) flush(w http.ResponseWriter) {
	maps.Copy(w.Header(), r.header)
	w.WriteHeader(r.code)
	if _, err := w.Write(r.buf.Bytes()); err != nil {
		return
	}
}
