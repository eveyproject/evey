package grpcapi

import (
	"context"
	"log/slog"
	"testing"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	domainplayback "github.com/gravity-zero/evey/internal/domain/playback"
	eveyi18n "github.com/gravity-zero/evey/internal/pkg/i18n"
)

func TestGetSystemStats(t *testing.T) {
	h := &adminHandler{startTime: time.Now().Add(-5 * time.Second)}
	resp, err := h.GetSystemStats(context.Background(), &eveyv1.GetSystemStatsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Goroutines <= 0 {
		t.Error("expected goroutines > 0")
	}
	if resp.UptimeSeconds < 4 {
		t.Errorf("expected uptime >= 4, got %d", resp.UptimeSeconds)
	}
	if resp.GoVersion == "" {
		t.Error("expected non-empty go version")
	}
	if resp.NumCpu <= 0 {
		t.Error("expected num_cpu > 0")
	}
}

func TestEnrichPeopleBios_NilEnricher(t *testing.T) {
	h := &adminHandler{}
	_, err := h.EnrichPeopleBios(context.Background(), &eveyv1.EnrichPeopleBiosRequest{})
	if err == nil {
		t.Fatal("expected error for nil enricher")
	}
}

type fakeEnricher struct{ called bool }

func (f *fakeEnricher) EnrichAllPeopleBios(_ context.Context, _ *slog.Logger) (int, error) {
	f.called = true
	return 0, nil
}

func TestEnrichPeopleBios_OK(t *testing.T) {
	h := &adminHandler{bioEnricher: &fakeEnricher{}, serverCtx: context.Background()}
	resp, err := h.EnrichPeopleBios(context.Background(), &eveyv1.EnrichPeopleBiosRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message != "enrichment started" {
		t.Errorf("unexpected message: %s", resp.Message)
	}
}

type fakeSessionProvider struct {
	sessions []*domainplayback.ActiveSessionInfo
	stopErr  error
}

func (f *fakeSessionProvider) GetActiveSessions(_ context.Context) ([]*domainplayback.ActiveSessionInfo, error) {
	return f.sessions, nil
}

func (f *fakeSessionProvider) StopStream(_ context.Context, _ string) error {
	return f.stopErr
}

func TestGetActiveSessions_Empty(t *testing.T) {
	h := &adminHandler{sessionProvider: &fakeSessionProvider{}}
	resp, err := h.GetActiveSessions(context.Background(), &eveyv1.GetActiveSessionsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(resp.Sessions))
	}
}

func TestGetActiveSessions_WithSessions(t *testing.T) {
	h := &adminHandler{sessionProvider: &fakeSessionProvider{
		sessions: []*domainplayback.ActiveSessionInfo{
			{SessionID: "s1", MediaTitle: "Test", StartedAt: time.Now(), DurationMs: 100000, PositionMs: 50000},
		},
	}}
	resp, err := h.GetActiveSessions(context.Background(), &eveyv1.GetActiveSessionsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(resp.Sessions))
	}
	if resp.Sessions[0].SessionId != "s1" {
		t.Errorf("expected session id s1, got %s", resp.Sessions[0].SessionId)
	}
}

func TestKillSession_OK(t *testing.T) {
	h := &adminHandler{sessionProvider: &fakeSessionProvider{}}
	_, err := h.KillSession(context.Background(), &eveyv1.KillSessionRequest{SessionId: "s1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestKillSession_EmptyID(t *testing.T) {
	h := &adminHandler{sessionProvider: &fakeSessionProvider{}}
	_, err := h.KillSession(context.Background(), &eveyv1.KillSessionRequest{})
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

func TestSetUserBitrateLimit_Negative(t *testing.T) {
	h := &adminHandler{translator: eveyi18n.NewEmpty()}
	_, err := h.SetUserBitrateLimit(context.Background(), &eveyv1.SetUserBitrateLimitRequest{UserId: "u1", MaxBitrateKbps: -1})
	if err == nil {
		t.Fatal("expected error for negative bitrate")
	}
}
