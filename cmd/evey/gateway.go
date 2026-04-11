package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/pkg/network"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

func registerGateway(grpcAddr string) (http.Handler, *grpc.ClientConn) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("grpc-gateway dial failed", slog.Any("err", err))
		return nil, nil
	}
	ctx := context.Background()
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{EmitUnpopulated: true},
		}),
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			switch strings.ToLower(key) {
			case "accept-language", "x-locale", "x-persistent-session":
				return strings.ToLower(key), true
			default:
				return runtime.DefaultHeaderMatcher(key)
			}
		}),
	)
	for _, fn := range []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
		eveyv1.RegisterLibraryServiceHandler,
		eveyv1.RegisterUserServiceHandler,
		eveyv1.RegisterPlaybackServiceHandler,
		eveyv1.RegisterTranscodingServiceHandler,
		eveyv1.RegisterMKVServiceHandler,
		eveyv1.RegisterMetadataServiceHandler,
		eveyv1.RegisterAdminSystemServiceHandler,
		eveyv1.RegisterAdminPluginServiceHandler,
		eveyv1.RegisterAdminTaskServiceHandler,
		eveyv1.RegisterAdminMediaServiceHandler,
		eveyv1.RegisterAdminSessionServiceHandler,
		eveyv1.RegisterActivityServiceHandler,
		eveyv1.RegisterNotificationServiceHandler,
		eveyv1.RegisterFeedServiceHandler,
		eveyv1.RegisterWebhookServiceHandler,
		eveyv1.RegisterCollectionServiceHandler,
		eveyv1.RegisterPlaylistServiceHandler,
		eveyv1.RegisterFavoriteServiceHandler,
		eveyv1.RegisterTranscodeCacheServiceHandler,
		eveyv1.RegisterBackupServiceHandler,
		eveyv1.RegisterRemoteAccessServiceHandler,
		eveyv1.RegisterPlaybackSettingsServiceHandler,
		eveyv1.RegisterLibraryAccessServiceHandler,
		eveyv1.RegisterServerConfigServiceHandler,
		eveyv1.RegisterSyncPlayServiceHandler,
		eveyv1.RegisterShareServiceHandler,
		eveyv1.RegisterRatingServiceHandler,
		eveyv1.RegisterMusicServiceHandler,
		eveyv1.RegisterPhotoServiceHandler,
		eveyv1.RegisterPushServiceHandler,
	} {
		if err := fn(ctx, mux, conn); err != nil {
			slog.Error("gateway register failed", slog.Any("err", err))
		}
	}
	return mux, conn
}

func networkMiddleware(detector *network.Detector, remoteEnabled *bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			nt := detector.Detect(r)
			if nt == network.WAN && !*remoteEnabled {
				http.Error(w, `{"message":"remote access disabled"}`, http.StatusForbidden)
				return
			}
			w.Header().Set("X-Evey-Network", string(nt))
			ctx := network.WithNetworkType(r.Context(), nt)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
