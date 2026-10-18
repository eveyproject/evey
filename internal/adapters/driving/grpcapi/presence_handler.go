package grpcapi

import (
	"context"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/app/presence"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
)

// presenceHandler serves PresenceService: an authenticated heartbeat that marks
// the caller online, and an admin read of everyone currently connected.
type presenceHandler struct {
	eveyv1.UnimplementedPresenceServiceServer
	tracker    *presence.Tracker
	translator *eveyi18n.Translator
}

func (h *presenceHandler) Heartbeat(ctx context.Context, _ *eveyv1.HeartbeatRequest) (*eveyv1.HeartbeatResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.UserID == "" {
		return nil, h.translator.TranslatedError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated")
	}
	h.tracker.Touch(presence.Entry{
		UserID:    claims.UserID,
		Username:  claims.Username,
		ProfileID: claims.ProfileID,
		IsAdmin:   claims.IsAdmin,
		ClientIP:  interceptors.ClientIPFromCtx(ctx),
		UserAgent: interceptors.UserAgentFromCtx(ctx),
	})
	return &eveyv1.HeartbeatResponse{OnlineCount: int32(h.tracker.Count())}, nil
}

func (h *presenceHandler) ListOnline(_ context.Context, _ *eveyv1.ListOnlineRequest) (*eveyv1.ListOnlineResponse, error) {
	now := time.Now()
	entries := h.tracker.Online()
	users := make([]*eveyv1.OnlineUser, 0, len(entries))
	for _, e := range entries {
		idle := max(int32(now.Sub(e.LastSeen).Seconds()), 0)
		users = append(users, &eveyv1.OnlineUser{
			UserId:       e.UserID,
			Username:     e.Username,
			ProfileId:    e.ProfileID,
			IsAdmin:      e.IsAdmin,
			ClientIp:     e.ClientIP,
			UserAgent:    e.UserAgent,
			LastSeenUnix: e.LastSeen.Unix(),
			IdleSeconds:  idle,
			// Derived here rather than in the panel: the product user agent is
			// the only thing that says "this device is a television", and the
			// stop control must not stop appearing because a UI copy of that
			// string drifted.
			IsCastReceiver: presence.IsCastReceiver(e.UserAgent),
		})
	}
	// Users has one row per DEVICE; Total stays the distinct-USER figure the
	// panel's "N users online" header wants (an account on web + mobile is 1).
	return &eveyv1.ListOnlineResponse{Users: users, Total: int32(h.tracker.Count())}, nil
}
