package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/network"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
)

func registerGateway(grpcAddr string) (http.Handler, *grpc.ClientConn) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		// Boot-time dial, before any request context exists.
		slog.Error("grpc-gateway dial failed", slog.Any("err", err))
		return nil, nil
	}
	ctx := context.Background()
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{EmitUnpopulated: true}, //nolint:modernize // embedlit: keep the embedded MarshalOptions explicit, it decides the JSON of every API answer and no test pins it
		}),
		runtime.WithIncomingHeaderMatcher(gatewayHeaderMatcher),
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
			slog.ErrorContext(ctx, "gateway register failed", slog.Any("err", err))
		}
	}
	return mux, conn
}

// gatewayOwnedMetadata are the keys only this gateway (or grpc-gateway itself)
// may set: the gRPC side trusts them because they come from a trusted peer.
var gatewayOwnedMetadata = map[string]bool{
	"x-real-ip": true, "x-forwarded-for": true, "x-forwarded-host": true,
	"x-real-user-agent": true, "x-request-id": true,
}

// gatewayHeaderMatcher maps HTTP headers to gRPC metadata. A client must never
// smuggle a gateway-owned key through Grpc-Metadata-*: grpc-gateway puts header
// pairs BEFORE WithMetadata ones, so a forged x-real-ip used to win.
func gatewayHeaderMatcher(key string) (string, bool) {
	k := strings.ToLower(key)
	switch k {
	case "accept-language", "x-locale", "x-persistent-session":
		return k, true
	}
	if inner, ok := strings.CutPrefix(k, strings.ToLower(runtime.MetadataHeaderPrefix)); ok &&
		(gatewayOwnedMetadata[inner] || strings.HasPrefix(inner, runtime.MetadataPrefix)) {
		return "", false
	}
	md, ok := runtime.DefaultHeaderMatcher(key)
	return strings.ToLower(md), ok
}

// remoteAccessRefusalCode is the stable token clients map onto "remote access is
// disabled on this server". Prose in `message` is for humans and changes with
// translations; this does not.
const remoteAccessRefusalCode = "remote_access_disabled"

// remoteAccessRefusalLogTTL bounds how often ONE client's refusal is logged. An
// app that retries on a schedule must not bury the log, but the very first
// refusal has to be visible immediately - a silent gate is what made the
// 2026-07-29 lockout undiagnosable.
const remoteAccessRefusalLogTTL = 5 * time.Minute

// networkGateExempt reports whether path must be served even to a client the
// remote-access gate refuses.
//
// /healthz: liveness - a gated server is not a dead server.
// /api/v1/i18n/: the translation catalogue. Refusing it is what turned an
// explainable "remote access is off" into an app rendering RAW KEYS
// ("auth.login.signIn") on a beta tester's phone - the operator then hunts a
// broken app instead of reading a message. A catalogue is public text; gating it
// protects nothing and destroys the only channel left to explain the refusal.
// /api/v1/setup/status: server identity/language, which the pre-auth screens
// need to render at all (and which clients use to tell an Evey server from a
// captive portal).
//
// Nothing here leaks library content or accepts credentials.
func networkGateExempt(path string) bool {
	return path == "/healthz" ||
		path == "/api/v1/setup/status" ||
		strings.HasPrefix(path, "/api/v1/i18n/")
}

// remoteEnabled is read on every request and flipped by an admin PUT at any
// moment, so it is a locked ACCESSOR, never a pointer to the config field: a
// raw *bool here read the flag with no synchronization at all while the REST
// and gRPC handlers wrote it under two different private mutexes.
func networkMiddleware(detector func() *network.Detector, remoteEnabled func() bool, logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	var (
		mu         sync.Mutex
		lastLogged = map[string]time.Time{}
	)
	// shouldLog reports whether this client's refusal has not been logged within
	// the TTL. Bounded: the map is swept whenever it grows past a scanner-sized
	// number of distinct clients.
	shouldLog := func(ip string) bool {
		now := time.Now()
		mu.Lock()
		defer mu.Unlock()
		if last, ok := lastLogged[ip]; ok && now.Sub(last) < remoteAccessRefusalLogTTL {
			return false
		}
		lastLogged[ip] = now
		if len(lastLogged) > 1024 {
			for k, t := range lastLogged {
				if now.Sub(t) >= remoteAccessRefusalLogTTL {
					delete(lastLogged, k)
				}
			}
		}
		return true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if networkGateExempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			nt := detector().Detect(r)
			if nt == network.WAN && !remoteEnabled() {
				clientIP := "unknown"
				if ip := network.ClientIP(r); ip != nil {
					clientIP = ip.String()
				}
				if shouldLog(clientIP) {
					logger.WarnContext(r.Context(), "remote access disabled: refused a client outside the local network (enable it in Admin > Remote access if this is a published server)",
						slog.String("client_ip", clientIP),
						slog.String("path", r.URL.Path),
						slog.String("method", r.Method),
						slog.String("device", r.UserAgent()))
				}
				// Counted as well as logged: the log line is rate limited per
				// address (so a retrying app cannot bury it), which is right for
				// reading but useless for noticing. A counter is what turns "one
				// client had a bad day" into "the whole household is locked out".
				metrics.RemoteAccessRefusedTotal.Inc()
				// Header first: it survives a client that never parses the body,
				// and it makes the cause visible in a plain curl -I.
				w.Header().Set("X-Evey-Remote-Access", "disabled")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"code":"` + remoteAccessRefusalCode + `","message":"remote access disabled"}`))
				return
			}
			w.Header().Set("X-Evey-Network", string(nt))
			ctx := network.WithNetworkType(r.Context(), nt)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
