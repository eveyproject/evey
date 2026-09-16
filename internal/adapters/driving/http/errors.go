package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// messageError writes {"message": msg} with the given status. Use it for any
// message that may carry user data or quotes: string concatenation into a JSON
// literal breaks the body as soon as the text contains a `"`.
func messageError(w http.ResponseWriter, msg string, code int) {
	body, err := json.Marshal(map[string]string{"message": msg})
	if err != nil {
		body = []byte(`{"message":"internal error"}`)
	}
	http.Error(w, string(body), code)
}

// internalError logs the real error server-side (correlated to the
// request_id / trace_id the logging middleware's contextHandler injects into
// ctx) and writes a generic 500 to the client. It is the HTTP-side
// equivalent of grpcapi's internalError: without ctx, an ERROR raised on this
// path was unreachable from the request_id an operator holds - the gRPC path
// had this since it was written, the direct HTTP handlers did not.
//
// attrs are extra structured fields (e.g. media_id) the caller wants
// attached to the log line; err is always logged under the "err" key.
func internalError(ctx context.Context, w http.ResponseWriter, op string, err error, attrs ...slog.Attr) {
	logInternal(ctx, op, err, attrs...)
	http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
}

// logInternal is internalError without the response: for paths that have
// already written headers (a streaming handler) and can only log.
//
// A caller that went away cancels the context, and every in-flight read fails
// with that cancellation. Logged as ERROR it is indistinguishable from a real
// fault - and it dominates: every ERROR raised on the HTTP side in the last
// three prod days was a client abort, which is exactly the noise that hides
// the one line an operator needs. Demoted to DEBUG, mirroring what grpcapi's
// internalError has always done.
func logInternal(ctx context.Context, op string, err error, attrs ...slog.Attr) {
	args := make([]any, 0, len(attrs)+1)
	args = append(args, slog.Any("err", err))
	for _, a := range attrs {
		args = append(args, a)
	}
	if isClientCanceled(err) {
		slog.DebugContext(ctx, op+": client canceled", args...)
		return
	}
	slog.ErrorContext(ctx, op, args...)
}

// isClientCanceled reports whether err is the caller going away rather than a
// server fault. Mirrors grpcapi.isClientCanceled, including the lib/pq
// projection of a cancellation (SQLSTATE 57014), which does not wrap
// context.Canceled.
func isClientCanceled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), "57014")
}
