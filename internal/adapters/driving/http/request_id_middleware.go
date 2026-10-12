package httpapi

import (
	"fmt"
	"net/http"
	"time"

	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

// RequestIDMiddleware injects a request ID into the context and response header.
// Reuses inbound X-Request-ID if present and well-formed (<=128 chars, printable),
// otherwise generates a new one.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := eveylogger.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	v, err := uuid.NewRandom()
	if err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", v[:8])
}

func validRequestID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}
