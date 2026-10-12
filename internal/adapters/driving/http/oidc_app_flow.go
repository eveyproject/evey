package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// SSO for the mobile app: the tokens wait server-side until redeemed with the
// poll token (sent to the app at start) AND the one-time code (sent through the
// evey:// deep link). A phishing link or a hijacked deep link holds only one.
const (
	oidcAppRedirect = "evey://oidc-callback"
	oidcAppFlowTTL  = 10 * time.Minute
	oidcAppFlowMax  = 1000
)

type oidcAppFlow struct {
	pollHash [32]byte
	code     string
	access   string
	refresh  string
	done     bool
	expires  time.Time
}

type oidcAppFlows struct {
	mu     sync.Mutex
	byID   map[string]*oidcAppFlow
	byCode map[string]string
	now    func() time.Time
}

func newOIDCAppFlows() *oidcAppFlows {
	return &oidcAppFlows{byID: map[string]*oidcAppFlow{}, byCode: map[string]string{}, now: time.Now}
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// start opens a flow; ok=false when too many are pending.
func (f *oidcAppFlows) start() (id, poll string, ok bool, err error) {
	var idBuf [16]byte
	if _, err = rand.Read(idBuf[:]); err != nil {
		return "", "", false, err
	}
	if poll, err = randomToken(32); err != nil {
		return "", "", false, err
	}
	id = hex.EncodeToString(idBuf[:])
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruneLocked()
	if len(f.byID) >= oidcAppFlowMax {
		return "", "", false, nil
	}
	f.byID[id] = &oidcAppFlow{pollHash: sha256.Sum256([]byte(poll)), expires: f.now().Add(oidcAppFlowTTL)}
	return id, poll, true, nil
}

func (f *oidcAppFlows) pruneLocked() {
	now := f.now()
	for id, fl := range f.byID {
		if now.After(fl.expires) {
			delete(f.byCode, fl.code)
			delete(f.byID, id)
		}
	}
}

func (f *oidcAppFlows) liveLocked(id string) *oidcAppFlow {
	fl, ok := f.byID[id]
	if !ok || f.now().After(fl.expires) {
		return nil
	}
	return fl
}

// pending reports a flow that exists and has not finished.
func (f *oidcAppFlows) pending(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.liveLocked(id)
	return fl != nil && !fl.done
}

// fail closes an app flow; false when id is not one (a web login).
func (f *oidcAppFlows) fail(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.liveLocked(id)
	if fl == nil {
		return false
	}
	fl.done = true
	return true
}

// complete parks the tokens of an app flow and returns the one-time code for
// the deep link; ok=false when id is not a pending app flow.
func (f *oidcAppFlows) complete(id, access, refresh string) (string, bool) {
	code, err := randomToken(32)
	if err != nil {
		return "", false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	fl := f.liveLocked(id)
	if fl == nil || fl.done {
		return "", false
	}
	fl.done, fl.code, fl.access, fl.refresh = true, code, access, refresh
	f.byCode[code] = id
	return code, true
}

// redeem hands the tokens over once. A known code burns the flow whatever the
// poll token, so neither secret can be guessed against it.
func (f *oidcAppFlows) redeem(code, poll string) (access, refresh string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, known := f.byCode[code]
	if !known || code == "" {
		return "", "", false
	}
	delete(f.byCode, code)
	fl := f.liveLocked(id)
	delete(f.byID, id)
	if fl == nil {
		return "", "", false
	}
	got := sha256.Sum256([]byte(poll))
	if subtle.ConstantTimeCompare(got[:], fl.pollHash[:]) != 1 {
		return "", "", false
	}
	return fl.access, fl.refresh, true
}

func registerOIDCAppRoutes(mux *http.ServeMux, oidcSvc *appuser.OIDCService, flows *oidcAppFlows, limited func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("POST /api/v1/auth/oidc/app/start", limited(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.auth.oidc.AppStart")
		defer span.End()
		if ok, _ := oidcSvc.Available(ctx); !ok {
			span.SetStatus(codes.Error, "sso unavailable")
			messageError(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		id, poll, ok, err := flows.start()
		if err != nil {
			span.RecordError(err)
			logInternal(ctx, "oidc app start", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if !ok {
			span.SetStatus(codes.Error, "too many pending flows")
			messageError(w, "too many pending sign-ins", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(map[string]string{
			"loginPath": "/api/v1/auth/oidc/login?flow=" + id,
			"pollToken": poll,
		}); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}))

	mux.HandleFunc("POST /api/v1/auth/oidc/app/token", limited(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.auth.oidc.AppToken")
		defer span.End()
		var body struct {
			Code      string `json:"code"`
			PollToken string `json:"pollToken"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			span.SetStatus(codes.Error, "bad request")
			messageError(w, "bad request", http.StatusBadRequest)
			return
		}
		access, refresh, ok := flows.redeem(body.Code, body.PollToken)
		if !ok {
			span.SetStatus(codes.Error, "invalid code")
			slog.WarnContext(ctx, "oidc app token: invalid or expired code")
			messageError(w, "invalid or expired code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(map[string]string{"accessToken": access, "refreshToken": refresh}); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}))
}
