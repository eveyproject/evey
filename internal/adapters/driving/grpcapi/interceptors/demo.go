package interceptors

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/evey-project/evey/internal/pkg/demo"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// DemoInterceptor refuses, on a demo instance, every RPC the demo policy
// locks. It sits AFTER the auth interceptor so an anonymous caller still gets
// Unauthenticated (the lock is not a probe), and it covers both transports:
// the grpc-gateway calls this same listener back over loopback.
//
// Zero `if demo` lives in a handler. The policy is internal/pkg/demo, the RBAC
// map says which methods are admin, and the contractguard test refuses an
// admin RPC that the policy does not classify.
type DemoInterceptor struct {
	rbac       RBACConfig
	translator *eveyi18n.Translator
}

func NewDemoInterceptor(rbac RBACConfig, translator *eveyi18n.Translator) *DemoInterceptor {
	return &DemoInterceptor{rbac: rbac, translator: translator}
}

func (d *DemoInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := d.check(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (d *DemoInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := d.check(ss.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (d *DemoInterceptor) check(ctx context.Context, method string) error {
	role, classified := d.rbac.MethodRoles[method]
	if !demo.RPCLocked(method, classified && role == RoleAdmin) {
		return nil
	}
	metrics.AuthzDeniedTotal.WithLabelValues("grpc", "demo_locked").Inc()
	slog.InfoContext(ctx, "demo: operation locked", slog.String("method", method))
	if d.translator != nil {
		return d.translator.TranslatedError(ctx, codes.PermissionDenied, demo.ErrorKey)
	}
	return status.Error(codes.PermissionDenied, demo.ErrorKey)
}
