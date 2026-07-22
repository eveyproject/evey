package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/pkg/network"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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
		// Capture the REAL client user-agent + IP into metadata. Without this,
		// downstream sees grpc-go's own user-agent and the gateway->grpc peer
		// (a private container IP) - useless in the audit log. ratelimit.ClientIP
		// only trusts X-Forwarded-For from a trusted proxy (anti-spoof).
		runtime.WithMetadata(func(_ context.Context, r *http.Request) metadata.MD {
			pairs := make([]string, 0, 4)
			if ua := r.Header.Get("User-Agent"); ua != "" {
				pairs = append(pairs, "x-real-user-agent", ua)
			}
			if ip := ratelimit.ClientIP(r); ip != "" {
				pairs = append(pairs, "x-real-ip", ip)
			}
			// Carry the HTTP leg's request id across to gRPC. Without it the
			// interceptor mints a fresh one, so the two halves of a single user
			// action are logged under different ids and cannot be joined - the
			// id we hand back in the response header would only ever match the
			// HTTP half.
			if id := r.Header.Get("X-Request-ID"); id != "" {
				pairs = append(pairs, "x-request-id", id)
			}
			return metadata.Pairs(pairs...)
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
		eveyv1.RegisterPresenceServiceHandler,
		eveyv1.RegisterAdminPluginServiceHandler,
		eveyv1.RegisterPluginUserDataServiceHandler,
		eveyv1.RegisterAdminTaskServiceHandler,
		eveyv1.RegisterAdminMediaServiceHandler,
		eveyv1.RegisterAdminIntroServiceHandler,
		eveyv1.RegisterAdminSessionServiceHandler,
		eveyv1.RegisterActivityServiceHandler,
		eveyv1.RegisterNotificationServiceHandler,
		eveyv1.RegisterFeedServiceHandler,
		eveyv1.RegisterWebhookServiceHandler,
		eveyv1.RegisterCollectionServiceHandler,
		eveyv1.RegisterPlaylistServiceHandler,
		eveyv1.RegisterFavoriteServiceHandler,
		eveyv1.RegisterMediaRequestServiceHandler,
		eveyv1.RegisterAdminMediaRequestServiceHandler,
		eveyv1.RegisterTranscodeCacheServiceHandler,
		eveyv1.RegisterBackupServiceHandler,
		eveyv1.RegisterBackupScheduleServiceHandler,
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
