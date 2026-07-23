package interceptors

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
)

// The HTTP middleware assigns a request id and hands it back in the response
// header, but the gRPC interceptor used to mint its own unconditionally. One
// user action therefore produced two unrelated ids, and the one the user could
// quote matched only the HTTP half of the logs.
func TestRequestIDFor_ReusesTheInboundID(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "abc123"))
	if got := requestIDFor(ctx); got != "abc123" {
		t.Errorf("inbound request id must be reused, got %q", got)
	}
}

func TestRequestIDFor_MintsOneWhenAbsent(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"no metadata": context.Background(),
		"no header":   metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "x")),
		"empty value": metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "")),
	} {
		t.Run(name, func(t *testing.T) {
			if got := requestIDFor(ctx); got == "" {
				t.Error("a request id must always be produced")
			}
		})
	}
}

// The id is written verbatim into log lines, so an attacker-controlled header
// must not be able to inject newlines or smuggle an unbounded blob into them.
func TestRequestIDFor_RejectsUnsafeInboundValues(t *testing.T) {
	for name, bad := range map[string]string{
		"newline injection": "abc\nlevel=ERROR msg=\"forged line\"",
		"carriage return":   "abc\rdef",
		"control byte":      "abc\x00def",
		"over 128 chars":    strings.Repeat("a", 129),
	} {
		t.Run(name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", bad))
			got := requestIDFor(ctx)
			if got == bad {
				t.Errorf("unsafe inbound id must be replaced, got %q", got)
			}
			if got == "" {
				t.Error("a replacement id must still be produced")
			}
		})
	}
}

// The request id must be stamped FIRST in the chain: recovery, sanitizer and
// rate-limit run before LoggingUnary, and their logs carried no request_id
// (migrating their log calls to *Context was a no-op - the id landed 6
// interceptors too late).
func TestRequestIDUnary_StampsBeforeDownstream(t *testing.T) {
	interceptor := RequestIDUnary()
	var seen string
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x"},
		func(ctx context.Context, _ any) (any, error) {
			seen = eveylogger.RequestIDFromCtx(ctx)
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if seen == "" {
		t.Fatal("downstream must observe a request id")
	}
}

func TestRequestIDUnary_ReusesGatewayHeader(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-request-id", "gw-42"))
	interceptor := RequestIDUnary()
	var seen string
	_, _ = interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/x"},
		func(ctx context.Context, _ any) (any, error) {
			seen = eveylogger.RequestIDFromCtx(ctx)
			return nil, nil
		})
	if seen != "gw-42" {
		t.Fatalf("must reuse the gateway id, got %q", seen)
	}
}

// LoggingUnary must NOT overwrite the id the first interceptor stamped: with
// no x-request-id header, requestIDFor mints a RANDOM id per call, and a
// second mint would split one request across two ids in the logs.
func TestLoggingUnary_KeepsAnAlreadyStampedID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chain := RequestIDUnary()
	logging := LoggingUnary(logger)
	var first, second string
	_, _ = chain(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x"},
		func(ctx context.Context, req any) (any, error) {
			first = eveylogger.RequestIDFromCtx(ctx)
			return logging(ctx, req, &grpc.UnaryServerInfo{FullMethod: "/x"},
				func(ctx context.Context, _ any) (any, error) {
					second = eveylogger.RequestIDFromCtx(ctx)
					return nil, nil
				})
		})
	if first == "" || first != second {
		t.Fatalf("logging must keep the stamped id: first=%q second=%q", first, second)
	}
}
