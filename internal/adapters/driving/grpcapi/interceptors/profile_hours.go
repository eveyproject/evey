package interceptors

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/evey-project/evey/internal/domain/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// ProfileAccessGate says whether a profile may be used now (kids opening
// hours). Implemented by app/user.ProfileSchedules.
type ProfileAccessGate interface {
	Access(ctx context.Context, profileID string) (user.ProfileAccess, error)
}

// ProfileClosedReason is the ErrorInfo reason clients match on; the HTTP
// middleware answers with the same one.
const ProfileClosedReason = "PROFILE_CLOSED"

// openWhileClosed are the RPCs a closed kids profile still needs to leave:
// who am I, which profiles exist, switch (RefreshToken with a PIN proof) or
// sign out. Nothing that lists or plays media.
var openWhileClosed = map[string]bool{
	"/evey.v1.UserService/RefreshToken":   true,
	"/evey.v1.UserService/Logout":         true,
	"/evey.v1.UserService/GetCurrentUser": true,
	"/evey.v1.UserService/ListProfiles":   true,
}

// ProfileHoursInterceptor refuses every RPC from a kids profile outside its
// opening hours. It sits after the auth interceptor (claims are known) and
// covers the REST gateway too, which calls this listener back.
type ProfileHoursInterceptor struct {
	gate       ProfileAccessGate
	translator *eveyi18n.Translator
}

func NewProfileHoursInterceptor(gate ProfileAccessGate, translator *eveyi18n.Translator) *ProfileHoursInterceptor {
	return &ProfileHoursInterceptor{gate: gate, translator: translator}
}

func (p *ProfileHoursInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := p.check(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (p *ProfileHoursInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := p.check(ss.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

func (p *ProfileHoursInterceptor) check(ctx context.Context, method string) error {
	claims := ClaimsFromContext(ctx)
	if p.gate == nil || claims == nil || claims.ProfileID == "" || openWhileClosed[method] {
		return nil
	}
	acc, err := p.gate.Access(ctx, claims.ProfileID)
	if err == nil && acc.Open {
		return nil
	}
	if err != nil {
		// Fail closed: hours that cannot be read do not open the profile.
		slog.WarnContext(ctx, "profile hours unreadable, call refused", slog.String("profile_id", claims.ProfileID), slog.Any("err", err))
	}
	metrics.AuthzDeniedTotal.WithLabelValues("grpc", "profile_closed").Inc()
	return ProfileClosedError(ctx, p.translator, claims.ProfileID, acc.NextOpen)
}

// ProfileClosedError is the refusal: PermissionDenied, a translated message,
// and an ErrorInfo carrying the reason, the closed profile and the next opening.
func ProfileClosedError(ctx context.Context, translator *eveyi18n.Translator, profileID string, nextOpen time.Time) error {
	msg := "errors.profile.closed"
	if translator != nil {
		msg = translator.T(eveyi18n.LocaleFromCtx(ctx), msg)
	}
	meta := map[string]string{"profile_id": profileID}
	if !nextOpen.IsZero() {
		meta["next_open"] = nextOpen.UTC().Format(time.RFC3339)
	}
	st, err := status.New(codes.PermissionDenied, msg).WithDetails(&errdetails.ErrorInfo{
		Reason:   ProfileClosedReason,
		Domain:   "evey",
		Metadata: meta,
	})
	if err != nil {
		return status.Error(codes.PermissionDenied, msg)
	}
	return st.Err()
}
