package syncplay

import (
	"net/http/httptest"
	"testing"
)

func TestExtractWSToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		authHeader string
		protocol  string
		query     string
		want      string
	}{
		{
			name:       "Authorization header takes priority",
			authHeader: "Bearer header-token",
			protocol:   "Bearer.proto-token",
			query:      "query-token",
			want:       "header-token",
		},
		{
			name:      "Subprotocol used when no Authorization header",
			protocol:  "Bearer.proto-token",
			query:     "query-token",
			want:      "proto-token",
		},
		{
			name:  "Query param fallback when no header or subprotocol",
			query: "query-token",
			want:  "query-token",
		},
		{
			name: "Empty when nothing provided",
			want: "",
		},
		{
			name:       "Non-Bearer Authorization header ignored",
			authHeader: "Basic dXNlcjpwYXNz",
			query:      "query-token",
			want:       "query-token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", "/ws?token="+tc.query, nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.protocol != "" {
				req.Header.Set("Sec-WebSocket-Protocol", tc.protocol)
			}
			got := extractWSToken(req)
			if got != tc.want {
				t.Errorf("extractWSToken() = %q, want %q", got, tc.want)
			}
		})
	}
}
