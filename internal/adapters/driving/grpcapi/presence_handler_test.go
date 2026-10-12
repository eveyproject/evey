package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/app/presence"
	"github.com/eveyproject/evey/internal/domain/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPresenceHeartbeat_TouchesTrackerAndCounts(t *testing.T) {
	h := &presenceHandler{tracker: presence.NewTracker()}
	ctx := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "usr-a", Username: "alice"})

	resp, err := h.Heartbeat(ctx, &eveyv1.HeartbeatRequest{})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if resp.OnlineCount != 1 {
		t.Fatalf("want online_count 1, got %d", resp.OnlineCount)
	}

	online, err := h.ListOnline(ctx, &eveyv1.ListOnlineRequest{})
	if err != nil {
		t.Fatalf("ListOnline: %v", err)
	}
	if online.Total != 1 || len(online.Users) != 1 || online.Users[0].Username != "alice" {
		t.Fatalf("unexpected ListOnline: %+v", online)
	}
}

// One tracker row per device but user-level totals: the same account beating
// from the web app and the mobile app must list both devices while the "N
// users online" figure stays 1 (2026-07-26, mobile presence wiring).
func TestPresenceListOnline_TwoDevicesOneUser(t *testing.T) {
	h := &presenceHandler{tracker: presence.NewTracker()}
	claims := &user.Claims{UserID: "usr-a", Username: "alice"}
	webCtx := interceptors.WithUserAgent(
		interceptors.ContextWithClaims(context.Background(), claims), "Mozilla/5.0 Chrome/126")
	mobileCtx := interceptors.WithUserAgent(
		interceptors.ContextWithClaims(context.Background(), claims), "EveyMobile (Android 14; SM-G991B)")

	if _, err := h.Heartbeat(webCtx, &eveyv1.HeartbeatRequest{}); err != nil {
		t.Fatalf("web Heartbeat: %v", err)
	}
	resp, err := h.Heartbeat(mobileCtx, &eveyv1.HeartbeatRequest{})
	if err != nil {
		t.Fatalf("mobile Heartbeat: %v", err)
	}
	if resp.OnlineCount != 1 {
		t.Fatalf("online_count must be distinct users (1), got %d", resp.OnlineCount)
	}

	online, err := h.ListOnline(webCtx, &eveyv1.ListOnlineRequest{})
	if err != nil {
		t.Fatalf("ListOnline: %v", err)
	}
	if len(online.Users) != 2 {
		t.Fatalf("want one row per device (2), got %d", len(online.Users))
	}
	if online.Total != 1 {
		t.Fatalf("total must be distinct users (1), got %d", online.Total)
	}
}

func TestPresenceHeartbeat_RequiresAuth(t *testing.T) {
	h := &presenceHandler{tracker: presence.NewTracker()}
	_, err := h.Heartbeat(context.Background(), &eveyv1.HeartbeatRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

func TestRestartServer_UnavailableWhenNotWired(t *testing.T) {
	h := &adminSystemHandler{} // restartFn nil
	_, err := h.RestartServer(context.Background(), &eveyv1.RestartServerRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable when restart not wired, got %v", err)
	}
}

func TestRestartServer_TriggersRestartFn(t *testing.T) {
	prev := restartGraceDelay
	restartGraceDelay = time.Millisecond
	defer func() { restartGraceDelay = prev }()

	called := make(chan struct{}, 1)
	h := &adminSystemHandler{restartFn: func() { called <- struct{}{} }}

	resp, err := h.RestartServer(context.Background(), &eveyv1.RestartServerRequest{})
	if err != nil {
		t.Fatalf("RestartServer: %v", err)
	}
	if !resp.Restarting {
		t.Fatalf("want restarting=true")
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("restartFn was not invoked")
	}
}
