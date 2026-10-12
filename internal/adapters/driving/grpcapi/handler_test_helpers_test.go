package grpcapi

import (
	"context"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func ctxWithClaims(userID, profileID string, isAdmin bool) context.Context {
	tokenGen := userfakes.NewFakeTokenGenerator()
	token, _ := tokenGen.GenerateAccessToken(user.Claims{
		UserID:    userID,
		Username:  userID,
		IsAdmin:   isAdmin,
		ProfileID: profileID,
	})

	ai := interceptors.NewAuthInterceptor(tokenGen)
	interceptor := ai.Unary()

	md := metadata.Pairs("authorization", "Bearer "+token)
	inCtx := metadata.NewIncomingContext(context.Background(), md)

	info := &grpc.UnaryServerInfo{FullMethod: "/evey.v1.TestService/Test"}
	var resultCtx context.Context
	_, _ = interceptor(inCtx, nil, info, func(ctx context.Context, req any) (any, error) {
		resultCtx = ctx
		return nil, nil
	})
	return resultCtx
}
