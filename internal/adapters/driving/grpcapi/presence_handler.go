package grpcapi

import (
	"context"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/app/presence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// presenceHandler serves PresenceService: an authenticated heartbeat that marks
// the caller online, and an admin read of everyone currently connected.
type presenceHandler struct {
	eveyv1.UnimplementedPresenceServiceServer
	tracker *presence.Tracker
}

func (h *presenceHandler) Heartbeat(ctx context.Context, _ *eveyv1.HeartbeatRequest) (*eveyv1.HeartbeatResponse, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.UserID == "" {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
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
		idle := int32(now.Sub(e.LastSeen).Seconds())
		if idle < 0 {
			idle = 0
		}
		users = append(users, &eveyv1.OnlineUser{
			UserId:       e.UserID,
			Username:     e.Username,
			ProfileId:    e.ProfileID,
			IsAdmin:      e.IsAdmin,
			ClientIp:     e.ClientIP,
			UserAgent:    e.UserAgent,
			LastSeenUnix: e.LastSeen.Unix(),
			IdleSeconds:  idle,
		})
	}
	return &eveyv1.ListOnlineResponse{Users: users, Total: int32(len(users))}, nil
}
