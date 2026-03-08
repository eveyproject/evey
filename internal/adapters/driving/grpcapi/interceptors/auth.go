package interceptors

import (
	"context"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type claimsKey struct{}

var publicMethods = map[string]bool{
	"/evey.v1.UserService/Login":              true,
	"/evey.v1.UserService/RegisterWithInvite": true,
	"/evey.v1.AdminService/SetupWizard":       true,
	"/evey.v1.AdminService/GetSystemInfo":     true,
	"/evey.v1.UserService/CheckQuickConnect":  true,
}

type AuthInterceptor struct {
	tokenGen user.TokenGenerator
}

func NewAuthInterceptor(tokenGen user.TokenGenerator) *AuthInterceptor {
	return &AuthInterceptor{tokenGen: tokenGen}
}

func (i *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}
		newCtx, err := i.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

func (i *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if publicMethods[info.FullMethod] {
			return handler(srv, ss)
		}
		newCtx, err := i.authenticate(ss.Context())
		if err != nil {
			return err
		}
		wrapped := &wrappedStream{ServerStream: ss, ctx: newCtx}
		return handler(srv, wrapped)
	}
}

func (i *AuthInterceptor) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization")
	}
	token := strings.TrimPrefix(vals[0], "Bearer ")
	claims, err := i.tokenGen.ValidateAccessToken(token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return context.WithValue(ctx, claimsKey{}, claims), nil
}

func ClaimsFromContext(ctx context.Context) *user.Claims {
	v, _ := ctx.Value(claimsKey{}).(*user.Claims)
	return v
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}
