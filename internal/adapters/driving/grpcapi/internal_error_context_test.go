package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	eveylogger "github.com/eveyproject/evey/internal/pkg/logger"
)

// ctxSpyHandler records the context each log record was emitted with. The real
// logger's contextHandler pulls request_id and trace_id off exactly this
// context, so seeing the id here is what proves the correlation works end to
// end - asserting on the formatted line would only prove the string exists.
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

// internalError is the tail of ~200 gRPC failure paths. It used to log with
// slog.Error, which drops the context, so not one ERROR line in the product
// carried the request_id an operator would search by: they could match the
// interceptor's one-line summary and nothing else.
func TestInternalError_LogsWithTheRequestContext(t *testing.T) {
	spy := &ctxSpyHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(spy))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := eveylogger.WithRequestID(context.Background(), "req-42")
	err := internalError(ctx, "do thing", errors.New("boom"))

	if err == nil {
		t.Fatal("internalError must still return an error to the caller")
	}
	if spy.calls == 0 {
		t.Fatal("internalError logged nothing")
	}
	if spy.seenRequestID != "req-42" {
		t.Errorf("the failure log must carry the request id, got %q", spy.seenRequestID)
	}
}

// A client that hangs up mid-request is not a server fault: it is logged at
// DEBUG so it does not pollute the error stream, but it must still be
// correlatable.
func TestInternalError_ClientCancelKeepsTheRequestContext(t *testing.T) {
	spy := &ctxSpyHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(spy))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx := eveylogger.WithRequestID(context.Background(), "req-99")
	_ = internalError(ctx, "do thing", context.Canceled)

	if spy.seenRequestID != "req-99" {
		t.Errorf("the cancellation log must carry the request id, got %q", spy.seenRequestID)
	}
}
