package interceptors

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecoveryUnary_RecoversPanic(t *testing.T) {
	t.Parallel()
	interceptor := RecoveryUnary()
	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/Panic"},
		func(context.Context, any) (any, error) { panic("boom") },
	)
	if resp != nil {
		t.Errorf("resp = %v, want nil", resp)
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("code = %v, want Internal", status.Code(err))
	}
}

func TestRecoveryUnary_PassesThroughNormal(t *testing.T) {
	t.Parallel()
	interceptor := RecoveryUnary()
	resp, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/test/OK"},
		func(context.Context, any) (any, error) { return "ok", nil },
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want ok", resp)
	}
}

type panicStream struct{ grpc.ServerStream }

func TestRecoveryStream_RecoversPanic(t *testing.T) {
	t.Parallel()
	interceptor := RecoveryStream()
	err := interceptor(nil, panicStream{},
		&grpc.StreamServerInfo{FullMethod: "/test/StreamPanic"},
		func(any, grpc.ServerStream) error { panic("boom") },
	)
	if status.Code(err) != codes.Internal {
		t.Errorf("code = %v, want Internal", status.Code(err))
	}
}
