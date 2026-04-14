package user

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	ufakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

type queryTestDeps struct {
	users        *ufakes.FakeUserRepository
	profiles     *ufakes.FakeProfileRepository
	apiKeys      *ufakes.FakeAPIKeyRepository
	invites      *ufakes.FakeInviteLinkRepository
	quickConnect *ufakes.FakeQuickConnectRepository
	svc          *QueryService
}

func newQueryTestDeps() *queryTestDeps {
	users := ufakes.NewFakeUserRepository()
	profiles := ufakes.NewFakeProfileRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	quickConnect := ufakes.NewFakeQuickConnectRepository()
	_ = fakes.NewFakeEventBus()

	svc := NewQueryService(users, profiles, apiKeys, invites, quickConnect)
	return &queryTestDeps{users, profiles, apiKeys, invites, quickConnect, svc}
}

// --- CountAdmins ---

func TestQueryCountAdmins_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.users.Create(ctx, &user.User{ID: "u1", Username: "admin", IsAdmin: true, Enabled: true})

	count, err := d.svc.CountAdmins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 admin, got %d", count)
	}
}

func TestQueryCountAdmins_RepoError(t *testing.T) {
	d := newQueryTestDeps()
	d.users.Err = errors.New("db fail")
	_, err := d.svc.CountAdmins(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ListUsers ---

func TestQueryListUsers_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.users.Create(ctx, &user.User{ID: "u1", Username: "alice"})
	_ = d.users.Create(ctx, &user.User{ID: "u2", Username: "bob"})

	users, err := d.svc.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestQueryListUsers_RepoError(t *testing.T) {
	d := newQueryTestDeps()
	d.users.Err = errors.New("db fail")
	_, err := d.svc.ListUsers(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ListProfiles ---

func TestQueryListProfiles_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "Main"})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p2", UserID: "u1", Name: "Kids"})

	profiles, err := d.svc.ListProfiles(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestQueryListProfiles_RepoError(t *testing.T) {
	d := newQueryTestDeps()
	d.profiles.Err = errors.New("fail")
	_, err := d.svc.ListProfiles(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ListInviteLinks ---

func TestQueryListInviteLinks_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "t1", CreatedBy: "u1"})

	links, err := d.svc.ListInviteLinks(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
}

func TestQueryListInviteLinks_RepoError(t *testing.T) {
	d := newQueryTestDeps()
	d.invites.Err = errors.New("fail")
	_, err := d.svc.ListInviteLinks(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- CheckQuickConnect ---

func TestQueryCheckQuickConnect_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.quickConnect.Create(ctx, &user.QuickConnectRequest{ID: "qc1", Code: "ABC", Status: "pending"})

	req, err := d.svc.CheckQuickConnect(ctx, "ABC")
	if err != nil {
		t.Fatal(err)
	}
	if req.Code != "ABC" {
		t.Fatalf("expected code ABC, got %s", req.Code)
	}
}

func TestQueryCheckQuickConnect_NotFound(t *testing.T) {
	d := newQueryTestDeps()
	_, err := d.svc.CheckQuickConnect(context.Background(), "NOPE")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ListAPIKeys ---

func TestQueryListAPIKeys_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.apiKeys.Create(ctx, &user.APIKeyModel{ID: "k1", UserID: "u1", Name: "key1", KeyHash: "h"})

	keys, err := d.svc.ListAPIKeys(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
}

func TestQueryListAPIKeys_RepoError(t *testing.T) {
	d := newQueryTestDeps()
	d.apiKeys.Err = errors.New("fail")
	_, err := d.svc.ListAPIKeys(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- GetUserBitrateLimit ---

func TestQueryGetUserBitrateLimit_Happy(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.users.Create(ctx, &user.User{ID: "u1", Username: "alice", MaxBitrateKbps: 8000})

	limit, err := d.svc.GetUserBitrateLimit(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if limit != 8000 {
		t.Fatalf("expected 8000, got %d", limit)
	}
}

func TestQueryGetUserBitrateLimit_ZeroDefault(t *testing.T) {
	d := newQueryTestDeps()
	ctx := context.Background()
	_ = d.users.Create(ctx, &user.User{ID: "u1", Username: "alice"})

	limit, err := d.svc.GetUserBitrateLimit(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if limit != 0 {
		t.Fatalf("expected 0, got %d", limit)
	}
}

func TestQueryGetUserBitrateLimit_UserNotFound(t *testing.T) {
	d := newQueryTestDeps()
	_, err := d.svc.GetUserBitrateLimit(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error")
	}
}
