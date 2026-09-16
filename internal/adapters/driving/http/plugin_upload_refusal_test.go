package httpapi

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appplugin "github.com/evey-project/evey/internal/app/plugin"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// An uploaded archive the installer refuses for what it IS (a broken
// manifest) is the admin's problem: 400 with the reason, the way a blocked
// one already answers 403 with its issues. Only a real fault is a 500.
func TestPluginUpload_RefusalIs400WithReason(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "a1", IsAdmin: true})

	post := func(installErr error) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		registerPluginUploadRoute(mux, gen, &stubPluginManager{installErr: installErr}, ratelimit.New(100))
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, err := mw.CreateFormFile("plugin", "p.zip")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("zip"))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	w := post(fmt.Errorf("invalid manifest: %w: name is required", appplugin.ErrInvalidManifest))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "name is required") {
		t.Errorf("invalid manifest: %d %s", w.Code, w.Body.String())
	}
	w = post(errors.New("write plugin dir: disk full"))
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "disk full") {
		t.Errorf("a server fault stays a sanitized 500: %d %s", w.Code, w.Body.String())
	}
	w = post(&appplugin.PluginBlockedError{Issues: []string{"[CRITICAL] eval"}, Categories: []string{"dynamic-eval"}})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "dynamic-eval") {
		t.Errorf("blocked: %d %s", w.Code, w.Body.String())
	}
}
