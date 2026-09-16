package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appplayback "github.com/evey-project/evey/internal/app/playback"
	"github.com/evey-project/evey/internal/domain/playback"
	pbfakes "github.com/evey-project/evey/internal/domain/playback/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
)

func newTestAdminSessionHandler() (*adminSessionHandler, *pbfakes.FakePlaySessionRepository) {
	sessionRepo := pbfakes.NewFakePlaySessionRepository()
	progressRepo := pbfakes.NewFakeWatchProgressRepository()
	userRepo := userfakes.NewFakeUserRepository()
	profileRepo := userfakes.NewFakeProfileRepository()
	eventBus := sharedfakes.NewFakeEventBus()

	cmdSvc := appplayback.NewCommandService(progressRepo, sessionRepo, sessionRepo, userRepo, profileRepo, eventBus)
	querySvc := appplayback.NewQueryService(progressRepo, sessionRepo)

	return &adminSessionHandler{
		playbackCmd: cmdSvc,
		playbackQry: querySvc,
	}, sessionRepo
}

func TestAdminGetAuditLog(t *testing.T) {
	d := newTestSystemHandler()
	audit := &adminSessionHandler{activitySvc: d.activitySvc}
	resp, err := audit.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{Limit: 10})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 audit entries, got %d", resp.Total)
	}
}

func TestSendSessionCommand_NonAdminDenied(t *testing.T) {
	audit := &adminSessionHandler{}
	ctx := userCtx("usr-1", "prf-1")
	_, err := audit.SendSessionCommand(ctx, &eveyv1.SendSessionCommandRequest{SessionId: "s-1", Command: "pause"})
	assertCode(t, err, codes.PermissionDenied)
}

func TestSendSessionCommand_Unauthenticated(t *testing.T) {
	audit := &adminSessionHandler{}
	_, err := audit.SendSessionCommand(context.Background(), &eveyv1.SendSessionCommandRequest{SessionId: "s-1", Command: "pause"})
	assertCode(t, err, codes.Unauthenticated)
}

func TestAdminGetAuditLog_WithFilters(t *testing.T) {
	d := newTestSystemHandler()
	audit := &adminSessionHandler{activitySvc: d.activitySvc}
	resp, err := audit.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{
		Limit:        10,
		UserId:       "u-1",
		Action:       "login",
		ResourceType: "user",
	})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	for _, entry := range resp.Entries {
		if entry.UserId != "u-1" {
			t.Errorf("expected UserId u-1, got %q", entry.UserId)
		}
		if entry.Action != "login" {
			t.Errorf("expected Action login, got %q", entry.Action)
		}
		if entry.ResourceType != "user" {
			t.Errorf("expected ResourceType user, got %q", entry.ResourceType)
		}
	}
}

func TestListActiveSessions_Empty(t *testing.T) {
	h, _ := newTestAdminSessionHandler()
	resp, err := h.ListActiveSessions(context.Background(), &eveyv1.ListActiveSessionsRequest{})
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(resp.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(resp.Sessions))
	}
}

func TestListActiveSessions_Seeded(t *testing.T) {
	h, sessionRepo := newTestAdminSessionHandler()
	pos, err := shared.NewDurationMs(12000)
	if err != nil {
		t.Fatal(err)
	}
	sessionRepo.Create(context.Background(), &playback.PlaySession{
		ID:          "sess-1",
		ProfileID:   "profile-1",
		MediaItemID: "mi-1",
		DeviceName:  "Firefox",
		DeviceType:  "web",
		PlayMethod:  string(playback.DirectPlay),
		VideoCodec:  "h264",
		AudioCodec:  "aac",
		Resolution:  "1080p",
		StartedAt:   time.Now(),
		Position:    &pos,
	})
	sessionRepo.Create(context.Background(), &playback.PlaySession{
		ID:          "sess-2",
		ProfileID:   "profile-2",
		MediaItemID: "mi-2",
		PlayMethod:  string(playback.Transcode),
		VideoCodec:  "hevc",
		StartedAt:   time.Now(),
	})

	resp, err := h.ListActiveSessions(context.Background(), &eveyv1.ListActiveSessionsRequest{})
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}
	seen := map[string]*eveyv1.ActiveSession{}
	for _, s := range resp.Sessions {
		seen[s.SessionId] = s
	}
	if seen["sess-1"] == nil || seen["sess-2"] == nil {
		t.Fatalf("missing sessions in response: %v", seen)
	}
	if seen["sess-2"].TranscodeInfo == "" {
		t.Errorf("expected transcode info for sess-2, got empty")
	}
}

func TestKillSession_MissingSessionID(t *testing.T) {
	h, _ := newTestAdminSessionHandler()
	_, err := h.KillSession(adminCtx(), &eveyv1.KillSessionRequest{SessionId: ""})
	assertCode(t, err, codes.InvalidArgument)
}

func TestKillSession_NotFound(t *testing.T) {
	h, _ := newTestAdminSessionHandler()
	_, err := h.KillSession(adminCtx(), &eveyv1.KillSessionRequest{SessionId: "nonexistent"})
	assertCode(t, err, codes.NotFound)
}

func TestKillSession_HappyPath(t *testing.T) {
	h, sessionRepo := newTestAdminSessionHandler()
	sessionRepo.Create(context.Background(), &playback.PlaySession{
		ID:          "sess-kill",
		ProfileID:   "profile-1",
		MediaItemID: "mi-1",
		PlayMethod:  string(playback.DirectPlay),
		StartedAt:   time.Now(),
	})

	resp, err := h.KillSession(adminCtx(), &eveyv1.KillSessionRequest{SessionId: "sess-kill"})
	if err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}
}

// KillSession force-stops ANY viewer's playback from a session id alone. Its
// sibling SendSessionCommand - same handler, same admin power, same file -
// re-checks claims.IsAdmin in the handler; KillSession checked nothing and
// leaned entirely on the RBAC interceptor's MethodRoles entry.
//
// That is one deleted map line away from letting any authenticated user stop
// anyone's stream, and nothing would fail: the interceptor table is data, not
// a call site, so no compiler and no test complains when an entry vanishes.
// Defence in depth is exactly the sibling's rule - apply it to both doors.
func TestKillSession_NonAdminDenied(t *testing.T) {
	audit := &adminSessionHandler{}
	ctx := userCtx("usr-1", "prf-1")
	_, err := audit.KillSession(ctx, &eveyv1.KillSessionRequest{SessionId: "s-1"})
	assertCode(t, err, codes.PermissionDenied)
}

func TestKillSession_Unauthenticated(t *testing.T) {
	audit := &adminSessionHandler{}
	_, err := audit.KillSession(context.Background(), &eveyv1.KillSessionRequest{SessionId: "s-1"})
	assertCode(t, err, codes.Unauthenticated)
}

// SendCastReceiverCommand is the admin's lever on a receiver that is still
// beating with no session: admin-only, both fields required, and the command
// is what the receiver's next beacon consumes for that profile.
func TestSendCastReceiverCommand(t *testing.T) {
	h, _ := newTestAdminSessionHandler()
	admin := ctxWithClaims("adm-1", "prf-adm", true)

	if _, err := h.SendCastReceiverCommand(admin, &eveyv1.SendCastReceiverCommandRequest{ProfileId: "prf-tv", Command: "stop"}); err != nil {
		t.Fatal(err)
	}
	cmd, err := h.playbackCmd.ConsumeCastCommand(context.Background(), "prf-tv", "mi-1")
	if err != nil || cmd != "stop" {
		t.Errorf("consumed %q err=%v, want the queued stop", cmd, err)
	}
	// Consumed once: a second read finds nothing.
	if cmd, _ := h.playbackCmd.ConsumeCastCommand(context.Background(), "prf-tv", "mi-1"); cmd != "" {
		t.Errorf("command must be consumed once, got %q again", cmd)
	}

	_, err = h.SendCastReceiverCommand(ctxWithClaims("u-1", "prf-1", false), &eveyv1.SendCastReceiverCommandRequest{ProfileId: "prf-tv", Command: "stop"})
	assertCode(t, err, codes.PermissionDenied)
	_, err = h.SendCastReceiverCommand(admin, &eveyv1.SendCastReceiverCommandRequest{ProfileId: "", Command: "stop"})
	assertCode(t, err, codes.InvalidArgument)
	_, err = h.SendCastReceiverCommand(admin, &eveyv1.SendCastReceiverCommandRequest{ProfileId: "prf-tv", Command: ""})
	assertCode(t, err, codes.InvalidArgument)
	if _, err := h.SendCastReceiverCommand(context.Background(), &eveyv1.SendCastReceiverCommandRequest{ProfileId: "prf-tv", Command: "stop"}); err == nil {
		t.Error("no claims must be refused")
	}
}
