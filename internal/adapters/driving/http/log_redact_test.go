package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Whatever the server logged is downloadable by an admin, and an admin
// response is still a response: a bearer, a JWT or a ?token= that reached a
// log line must not come back out of the log routes in clear.
func TestRedactLogLine(t *testing.T) {
	const jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1MSJ9.c2lnbmF0dXJl"
	cases := []struct {
		name, in string
		gone     []string
		kept     []string
	}{
		{"bearer header", `{"msg":"proxy","authorization":"Bearer abc.def-ghi"}`, []string{"abc.def-ghi"}, []string{`"authorization":"Bearer [redacted]"`}},
		{"bare jwt anywhere", `stream url http://h/x.m3u8?jwt=` + jwt + ` failed`, []string{jwt}, []string{"stream url", "failed"}},
		{"query token", `GET /api/v1/media/m1/stream?token=s3cr3t&w=100 200`, []string{"s3cr3t"}, []string{"token=[redacted]", "&w=100", "200"}},
		{"query token in json", `{"path":"/hls/master.m3u8?token=abc123","status":200}`, []string{"abc123"}, []string{`token=[redacted]"`, `"status":200`}},
		{"refresh and api key params", `refresh_token=rt-1 api_key=ak-1 apikey=ak-2 access_token=at-1`, []string{"rt-1", "ak-1", "ak-2", "at-1"}, nil},
		{"password field", `{"msg":"login","password":"hunter2","username":"bob"}`, []string{"hunter2"}, []string{`"username":"bob"`}},
		{"slog password kv", `level=INFO msg=login password=hunter2 user=bob`, []string{"hunter2"}, []string{"user=bob"}},
		{"pin field", `{"pin":"1234","profile":"kid"}`, []string{"1234"}, []string{`"profile":"kid"`}},
		{"plain line untouched", `{"msg":"scan done","files":12,"token_count":3}`, nil, []string{`"token_count":3`, "scan done"}},
	}
	for _, c := range cases {
		out := redactLogLine(c.in)
		for _, g := range c.gone {
			if strings.Contains(out, g) {
				t.Errorf("%s: %q still contains %q", c.name, out, g)
			}
		}
		for _, k := range c.kept {
			if !strings.Contains(out, k) {
				t.Errorf("%s: %q lost %q", c.name, out, k)
			}
		}
	}
}

// The three routes serve the same file: tail, search and the SSE follow must
// all go through the scrub.
func TestLogRoutes_RedactCredentials(t *testing.T) {
	dir := t.TempDir()
	line := `{"time":"2026-09-16T00:00:00Z","level":"INFO","msg":"GET /api/v1/media/m1/stream?token=s3cr3t","authorization":"Bearer abc.def"}`
	if err := os.WriteFile(filepath.Join(dir, "evey-2026-09-16.log"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gen, adminTok, _ := genTokens(t)
	mux := http.NewServeMux()
	registerLogsRoute(mux, dir, gen, nil)

	get := func(path string) string {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+adminTok)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, path := range []string{"/api/v1/admin/logs", "/api/v1/admin/logs/search?q=stream"} {
		body := get(path)
		if strings.Contains(body, "s3cr3t") || strings.Contains(body, "abc.def") {
			t.Errorf("%s leaked a credential: %s", path, body)
		}
		if !strings.Contains(body, "token=[redacted]") {
			t.Errorf("%s: expected the redaction marker: %s", path, body)
		}
	}
}
