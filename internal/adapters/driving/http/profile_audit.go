package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	appactivity "github.com/eveyproject/evey/internal/app/activity"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// auditBodyPeek bounds how much of a JSON body is kept to name its fields.
const auditBodyPeek = 64 << 10

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// auditedRoute records a profile change in the audit log once next has
// answered, refused or not: who asked, on which {idParam}, the top-level
// field names of a JSON body (never their values) and the HTTP status. It
// sits inside the auth middleware, so the caller's claims are on the request.
func auditedRoute(svc *appactivity.CommandService, action, resourceType, idParam string, next http.HandlerFunc) http.HandlerFunc {
	if svc == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var peek bytes.Buffer
		if r.Body != nil && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			r.Body = struct {
				io.Reader
				io.Closer
			}{io.TeeReader(r.Body, &limitedBuffer{buf: &peek, max: auditBodyPeek}), r.Body}
		}
		rec := &statusRecorder{ResponseWriter: w}
		next(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		details := map[string]any{"status": status}
		if status < 400 {
			details["outcome"] = "success"
		} else {
			details["outcome"] = "refused"
		}
		if fields := jsonFieldNames(peek.Bytes()); len(fields) > 0 {
			details["fields"] = fields
		}
		e := &activity.AuditLogEntry{Action: action, ResourceType: resourceType, Details: details}
		if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
			e.UserID = shared.UserID(claims.UserID)
			e.ProfileID = shared.ProfileID(claims.ProfileID)
		}
		if id := r.PathValue(idParam); id != "" {
			e.ResourceID = &id
		}
		ip, ua := r.RemoteAddr, r.UserAgent()
		e.ClientIP, e.UserAgent = &ip, &ua
		if err := svc.CreateAuditEntry(r.Context(), e); err != nil {
			slog.WarnContext(r.Context(), "audit log failed", slog.String("action", action), slog.Any("err", err))
		}
	}
}

// limitedBuffer keeps the first max bytes written to it and drops the rest,
// so a large body still reaches the handler whole.
type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

// jsonFieldNames returns the sorted top-level keys of a JSON object, or nil.
func jsonFieldNames(body []byte) []string {
	var obj map[string]json.RawMessage
	if len(body) == 0 || json.Unmarshal(body, &obj) != nil {
		return nil
	}
	names := make([]string, 0, len(obj))
	for k := range obj {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
