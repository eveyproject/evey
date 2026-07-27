package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"

	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
)

// ctxSpyHandler records the context each log record was emitted with. The
// real logger's contextHandler pulls request_id off exactly this context, so
// seeing the id here is what proves the correlation works end to end.
type ctxSpyHandler struct {
	seenRequestID string
	calls         int
}

func (h *ctxSpyHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *ctxSpyHandler) Handle(ctx context.Context, _ slog.Record) error {
	h.calls++
	h.seenRequestID = eveylogger.RequestIDFromCtx(ctx)
	return nil
}

func (h *ctxSpyHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *ctxSpyHandler) WithGroup(string) slog.Handler      { return h }

// internalError is the tail of a growing number of direct HTTP handlers. It
// must log with the request's context so an operator can correlate an ERROR
// line back to the request_id logged at the edge - the gap this helper closes.
func TestInternalError_LogsWithTheRequestContextAndWrites500(t *testing.T) {
	spy := &ctxSpyHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(spy))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := eveylogger.WithRequestID(context.Background(), "req-http-1")
	w := httptest.NewRecorder()

	internalError(ctx, w, "do thing", errors.New("boom"), slog.String("media_id", "m-1"))

	if spy.calls == 0 {
		t.Fatal("internalError logged nothing")
	}
	if spy.seenRequestID != "req-http-1" {
		t.Errorf("the failure log must carry the request id, got %q", spy.seenRequestID)
	}
	if w.Code != 500 {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if got := w.Body.String(); got != `{"message":"internal error"}`+"\n" {
		t.Errorf("unexpected body: %q", got)
	}
}
