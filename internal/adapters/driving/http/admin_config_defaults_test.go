package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// The panel reads the real defaults (admins only): the configured language,
// the metadata language following a stored server language, the zone.
func TestConfigDefaults_AdminReadsTheEffectiveValues(t *testing.T) {
	gen, _, _ := genTokens(t)
	repo := sharedfakes.NewFakeServerConfigRepository()
	_ = repo.Set(context.Background(), "server.language", "es")
	t.Setenv("TZ", "Europe/Zurich")
	mux := http.NewServeMux()
	registerConfigDefaultsRoute(mux, gen, repo, "fr")

	user, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "p1"})
	if w := do(mux, http.MethodGet, "/api/v1/admin/config/defaults", user, "", nil); w.Code != http.StatusForbidden {
		t.Errorf("non-admin: %d, want 403", w.Code)
	}
	admin, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "a1", ProfileID: "pa", IsAdmin: true})
	w := do(mux, http.MethodGet, "/api/v1/admin/config/defaults", admin, "", nil)
	var got map[string]string
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatalf("admin: %d %s", w.Code, w.Body.String())
	}
	want := map[string]string{"server.language": "fr", "metadata.language": "es", "server.timezone": "Europe/Zurich"}
	if now, err := time.Parse(time.RFC3339, got["now"]); err != nil || time.Since(now) > time.Minute {
		t.Errorf("now = %q, want the server's current time", got["now"])
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}
