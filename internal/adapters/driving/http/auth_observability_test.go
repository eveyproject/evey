package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appuser "github.com/eveyproject/evey/internal/app/user"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// An unreadable token epoch fails EVERY authenticated request in the server at
// once. Reported as plain "invalid_token" it is indistinguishable from one
// user's expired session, and the incident runbook sends the operator to look
// at the JWT key - which is fine. The log has to say which of the two it is.
func TestAuthMiddleware_EpochLookupFailureIsNotReportedAsAnInvalidToken(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	gen := &epochFailingGen{}
	h := httpAuthMiddleware(gen)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (fail closed)", rec.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "epoch") {
		t.Errorf("the log does not name the epoch as the cause, so the operator cannot tell this from an expired token:\n%s", out)
	}
	if !strings.Contains(out, "database") {
		t.Errorf("the log does not point at the database:\n%s", out)
	}
}

// epochFailingGen returns the tagged error the middleware keys on.
type epochFailingGen struct{ domainuser.TokenGenerator }

func (epochFailingGen) ValidateAccessToken(string) (*domainuser.Claims, error) {
	return nil, errors.Join(appuser.ErrEpochUnavailable, errors.New("dial tcp: connection refused"))
}
