package httpapi

import (
	"context"
	"log/slog"
	"net/http"
)

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
	args := make([]any, 0, len(attrs)+1)
	args = append(args, slog.Any("err", err))
	for _, a := range attrs {
		args = append(args, a)
	}
	slog.ErrorContext(ctx, op, args...)
	http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
}
