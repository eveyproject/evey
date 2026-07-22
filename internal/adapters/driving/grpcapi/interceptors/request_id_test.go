package interceptors

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/metadata"
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
