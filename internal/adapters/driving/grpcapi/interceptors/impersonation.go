package interceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// forbiddenWhileImpersonating are the calls that would outlive "connect as":
// its token has no refresh and dies in minutes, but an API key or a device
// signed in by Quick Connect would keep the target's account open for the
// admin afterwards, and a second impersonation would hide who started it.
var forbiddenWhileImpersonating = map[string]bool{
	"/evey.v1.UserService/CreateAPIKey":          true,
	"/evey.v1.UserService/AuthorizeQuickConnect": true,
	"/evey.v1.UserService/ImpersonateUser":       true,
}

// ImpersonationGuard refuses, under an impersonation token, what would let
// the admin keep the target's access or change the target's own secrets (a
// profile PIN). Placed after the auth interceptor.
type ImpersonationGuard struct {
	translator *eveyi18n.Translator
}

func NewImpersonationGuard(translator *eveyi18n.Translator) *ImpersonationGuard {
	return &ImpersonationGuard{translator: translator}
}

func (g *ImpersonationGuard) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := g.check(ctx, info.FullMethod, req); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (g *ImpersonationGuard) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := g.check(ss.Context(), info.FullMethod, nil); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (g *ImpersonationGuard) check(ctx context.Context, method string, req any) error {
	claims := ClaimsFromContext(ctx)
	if claims == nil || claims.ImpersonatedBy == "" {
		return nil
	}
	forbidden := forbiddenWhileImpersonating[method]
	if p, ok := req.(*eveyv1.UpdateProfileRequest); ok && p.Pin != nil {
		forbidden = true
	}
	if !forbidden {
		return nil
	}
	metrics.AuthzDeniedTotal.WithLabelValues("grpc", "impersonation").Inc()
	msg := "errors.user.impersonation_forbidden"
	if g.translator != nil {
		msg = g.translator.T(eveyi18n.LocaleFromCtx(ctx), msg)
	}
	return status.Error(codes.PermissionDenied, msg)
}
