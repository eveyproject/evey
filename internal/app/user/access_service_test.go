package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	ufakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

type accessTestDeps struct {
	users        *ufakes.FakeUserRepository
	apiKeys      *ufakes.FakeAPIKeyRepository
	invites      *ufakes.FakeInviteLinkRepository
	quickConnect *ufakes.FakeQuickConnectRepository
	hasher       *ufakes.FakePasswordHasher
	events       *fakes.FakeEventBus
	svc          *AccessService
}

func newAccessTestDeps() *accessTestDeps {
	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	quickConnect := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	events := fakes.NewFakeEventBus()

	svc := NewAccessService(users, apiKeys, invites, quickConnect, hasher, events)
	return &accessTestDeps{users, apiKeys, invites, quickConnect, hasher, events, svc}
}

// --- CreateInviteLink ---

func TestCreateInviteLink_Happy(t *testing.T) {
	d := newAccessTestDeps()
	link := &user.InviteLink{ID: "inv1", Token: "tok", CreatedBy: "u1"}
	err := d.svc.CreateInviteLink(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}
	if link.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt set")
	}
}

func TestCreateInviteLink_ValidationError(t *testing.T) {
	d := newAccessTestDeps()
	err := d.svc.CreateInviteLink(context.Background(), &user.InviteLink{ID: "inv1", Token: "", CreatedBy: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCreateInviteLink_RepoError(t *testing.T) {
	d := newAccessTestDeps()
	d.invites.Err = errors.New("fail")
	err := d.svc.CreateInviteLink(context.Background(), &user.InviteLink{ID: "inv1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- RevokeInviteLink ---

func TestRevokeInviteLink_Happy(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})

	err := d.svc.RevokeInviteLink(ctx, "inv1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestRevokeInviteLink_RepoError(t *testing.T) {
	d := newAccessTestDeps()
	d.invites.Err = errors.New("fail")
	err := d.svc.RevokeInviteLink(context.Background(), "inv1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- RegisterWithInvite ---

func TestRegisterWithInvite_Happy(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "valid-token", MaxUses: 10, UsedCount: 0})

	u, err := d.svc.RegisterWithInvite(ctx, "valid-token", "bob", "longpassword", "bob@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "bob" || u.Email != "bob@b.com" {
		t.Fatal("wrong user fields")
	}
	if !u.Enabled {
		t.Fatal("expected enabled")
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.created" {
		t.Fatal("expected user.created event")
	}

	inv, _ := d.invites.GetByToken(ctx, "valid-token")
	if inv.UsedCount != 1 {
		t.Fatalf("expected UsedCount=1, got %d", inv.UsedCount)
	}
}

func TestRegisterWithInvite_ShortPassword(t *testing.T) {
	d := newAccessTestDeps()
	_, err := d.svc.RegisterWithInvite(context.Background(), "tok", "bob", "short", "b@b.com")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestRegisterWithInvite_InvalidToken(t *testing.T) {
	d := newAccessTestDeps()
	_, err := d.svc.RegisterWithInvite(context.Background(), "bad-token", "bob", "longpassword", "b@b.com")
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
}

func TestRegisterWithInvite_ExpiredInvite(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	expired := time.Now().Add(-1 * time.Hour)
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "exp-tok", ExpiresAt: &expired})

	_, err := d.svc.RegisterWithInvite(ctx, "exp-tok", "bob", "longpassword", "b@b.com")
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
}

func TestRegisterWithInvite_MaxUsesReached(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "max-tok", MaxUses: 1, UsedCount: 1})

	_, err := d.svc.RegisterWithInvite(ctx, "max-tok", "bob", "longpassword", "b@b.com")
	if !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expected ErrInviteInvalid, got %v", err)
	}
}

func TestRegisterWithInvite_MaxUsesBoundary(t *testing.T) {
	t.Run("UsedCount=MaxUses-1 succeeds", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok1", MaxUses: 3, UsedCount: 2})

		u, err := d.svc.RegisterWithInvite(ctx, "tok1", "bob", "longpassword", "b@b.com")
		if err != nil {
			t.Fatalf("UsedCount=MaxUses-1 should succeed, got %v", err)
		}
		if u == nil {
			t.Fatal("expected user")
		}
	})
	t.Run("UsedCount=MaxUses rejected", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok2", MaxUses: 3, UsedCount: 3})

		_, err := d.svc.RegisterWithInvite(ctx, "tok2", "bob", "longpassword", "b@b.com")
		if !errors.Is(err, ErrInviteInvalid) {
			t.Fatalf("UsedCount=MaxUses should be rejected, got %v", err)
		}
	})
	t.Run("UsedCount=MaxUses+1 rejected", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok3", MaxUses: 3, UsedCount: 4})

		_, err := d.svc.RegisterWithInvite(ctx, "tok3", "bob", "longpassword", "b@b.com")
		if !errors.Is(err, ErrInviteInvalid) {
			t.Fatalf("UsedCount>MaxUses should be rejected, got %v", err)
		}
	})
	t.Run("expiry boundary - just expired", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		justPast := time.Now().Add(-1 * time.Millisecond)
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok4", ExpiresAt: &justPast})

		_, err := d.svc.RegisterWithInvite(ctx, "tok4", "bob", "longpassword", "b@b.com")
		if !errors.Is(err, ErrInviteInvalid) {
			t.Fatalf("just-expired invite should be rejected, got %v", err)
		}
	})
	t.Run("expiry boundary - still valid", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		future := time.Now().Add(1 * time.Hour)
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok5", ExpiresAt: &future})

		u, err := d.svc.RegisterWithInvite(ctx, "tok5", "bob", "longpassword", "b@b.com")
		if err != nil {
			t.Fatalf("future expiry should succeed, got %v", err)
		}
		if u == nil {
			t.Fatal("expected user")
		}
	})
}

// Concurrent TOCTOU: N goroutines race to register on a single-use invite.
// Exactly one must succeed; the rest must fail with ErrInviteInvalid.
func TestRegisterWithInvite_TOCTOU_SingleUse(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "race-tok", MaxUses: 1})

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	results := make(chan error, N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("user-%d", i)
			email := fmt.Sprintf("u%d@example.com", i)
			_, err := d.svc.RegisterWithInvite(ctx, "race-tok", name, "longpassword", email)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	ok, rejected := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrInviteInvalid) {
			rejected++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("expected exactly 1 successful registration, got %d", ok)
	}
	if rejected != N-1 {
		t.Errorf("expected %d rejections with ErrInviteInvalid, got %d", N-1, rejected)
	}
}

func TestRegisterWithInvite_PasswordBoundary(t *testing.T) {
	t.Run("7 chars rejected", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
		_, err := d.svc.RegisterWithInvite(ctx, "tok", "bob", "1234567", "b@b.com")
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("7-char password should be rejected, got %v", err)
		}
	})
	t.Run("8 chars accepted", func(t *testing.T) {
		d := newAccessTestDeps()
		ctx := context.Background()
		_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
		u, err := d.svc.RegisterWithInvite(ctx, "tok", "bob", "12345678", "b@b.com")
		if errors.Is(err, ErrPasswordTooShort) {
			t.Fatal("8-char password should be accepted")
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if u == nil {
			t.Fatal("expected user")
		}
	})
}

func TestRegisterWithInvite_HashError(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
	d.hasher.Err = errors.New("hash fail")

	_, err := d.svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "b@b.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWithInvite_UserCreateError(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
	d.users.Err = errors.New("create fail")

	_, err := d.svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "b@b.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterWithInvite_AdminInvite(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "admin-tok", IsAdmin: true})

	u, err := d.svc.RegisterWithInvite(ctx, "admin-tok", "bob", "longpassword", "b@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin {
		t.Fatal("expected admin user from admin invite")
	}
}

func TestRegisterWithInvite_InviteWithNoExpiry(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "no-exp"})

	u, err := d.svc.RegisterWithInvite(ctx, "no-exp", "bob", "longpassword", "b@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("expected user")
	}
}

func TestRegisterWithInvite_UnlimitedUses(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "unlim", MaxUses: 0, UsedCount: 100})

	u, err := d.svc.RegisterWithInvite(ctx, "unlim", "bob", "longpassword", "b@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("expected user with unlimited invite")
	}
}

func TestRegisterWithInvite_EventPublishError_NonFatal(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
	d.events.PublishErr = errors.New("bus down")

	u, err := d.svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "b@b.com")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
	if u == nil {
		t.Fatal("expected user")
	}
}

// --- InitiateQuickConnect ---

func TestInitiateQuickConnect_Happy(t *testing.T) {
	d := newAccessTestDeps()
	req := &user.QuickConnectRequest{ID: "qc1", Code: "ABC"}
	err := d.svc.InitiateQuickConnect(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if req.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt set")
	}
}

func TestInitiateQuickConnect_RepoError(t *testing.T) {
	d := newAccessTestDeps()
	d.quickConnect.Err = errors.New("fail")
	err := d.svc.InitiateQuickConnect(context.Background(), &user.QuickConnectRequest{Code: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- AuthorizeQuickConnect ---

func TestAuthorizeQuickConnect_Happy(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.quickConnect.Create(ctx, &user.QuickConnectRequest{ID: "qc1", Code: "ABC", Status: "pending"})

	fullCode, err := d.svc.AuthorizeQuickConnect(ctx, "ABC", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if fullCode != "ABC" {
		t.Fatalf("expected full code ABC, got %s", fullCode)
	}
	req, _ := d.quickConnect.GetByCode(ctx, "ABC")
	if req.Status != "authorized" {
		t.Fatalf("expected authorized, got %s", req.Status)
	}
}

func TestAuthorizeQuickConnect_NotFound(t *testing.T) {
	d := newAccessTestDeps()
	_, err := d.svc.AuthorizeQuickConnect(context.Background(), "NOPE", "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- AuthorizeByShortCode ---

func TestAuthorizeByShortCode_Happy(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.quickConnect.Create(ctx, &user.QuickConnectRequest{ID: "qc1", Code: "qc-code-aabbccdd11223344", Status: "pending"})

	fullCode, err := d.svc.AuthorizeByShortCode(ctx, "223344", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if fullCode != "qc-code-aabbccdd11223344" {
		t.Fatalf("expected full code, got %s", fullCode)
	}
	req, _ := d.quickConnect.GetByCode(ctx, "qc-code-aabbccdd11223344")
	if req.Status != "authorized" {
		t.Fatalf("expected authorized, got %s", req.Status)
	}
}

func TestAuthorizeByShortCode_NotFound(t *testing.T) {
	d := newAccessTestDeps()
	_, err := d.svc.AuthorizeByShortCode(context.Background(), "XXXXXX", "u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- CreateAPIKey ---

func TestCreateAPIKey_Happy(t *testing.T) {
	d := newAccessTestDeps()
	key := &user.APIKeyModel{ID: "k1", UserID: "u1", Name: "test-key", KeyHash: "fakehash", Permissions: []string{"media:read"}}
	err := d.svc.CreateAPIKey(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if key.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt set")
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.apikey.created" {
		t.Fatal("expected apikey.created event")
	}
}

func TestCreateAPIKey_ValidationError(t *testing.T) {
	d := newAccessTestDeps()
	err := d.svc.CreateAPIKey(context.Background(), &user.APIKeyModel{ID: "k1", UserID: "", Name: "", KeyHash: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCreateAPIKey_RepoError(t *testing.T) {
	d := newAccessTestDeps()
	d.apiKeys.Err = errors.New("fail")
	err := d.svc.CreateAPIKey(context.Background(), &user.APIKeyModel{ID: "k1", UserID: "u1", Name: "test", KeyHash: "h", Permissions: []string{"media:read"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateAPIKey_EventPublishError_NonFatal(t *testing.T) {
	d := newAccessTestDeps()
	d.events.PublishErr = errors.New("bus down")
	err := d.svc.CreateAPIKey(context.Background(), &user.APIKeyModel{ID: "k1", UserID: "u1", Name: "k", KeyHash: "fakehash", Permissions: []string{"media:read"}})
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- RevokeAPIKey ---

func TestRevokeAPIKey_Happy(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.apiKeys.Create(ctx, &user.APIKeyModel{ID: "k1", UserID: "u1"})

	err := d.svc.RevokeAPIKey(ctx, "k1")
	if err != nil {
		t.Fatal(err)
	}

	evts := d.events.PublishedEvents()
	if len(evts) != 1 || evts[0].EventName() != "user.apikey.revoked" {
		t.Fatal("expected apikey.revoked event")
	}
}

func TestRevokeAPIKey_RepoError(t *testing.T) {
	d := newAccessTestDeps()
	d.apiKeys.Err = errors.New("fail")
	err := d.svc.RevokeAPIKey(context.Background(), "k1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRevokeAPIKey_EventPublishError_NonFatal(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.apiKeys.Create(ctx, &user.APIKeyModel{ID: "k1", UserID: "u1"})
	d.events.PublishErr = errors.New("bus down")

	err := d.svc.RevokeAPIKey(ctx, "k1")
	if err != nil {
		t.Fatalf("event error should be non-fatal, got %v", err)
	}
}

// --- RegisterWithInvite with profiles ---

func TestRegisterWithInvite_WithProfiles_CreatesDefaultProfile(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	quickConnect := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	profiles := ufakes.NewFakeProfileRepository()
	events := fakes.NewFakeEventBus()

	svc := NewAccessService(users, apiKeys, invites, quickConnect, hasher, events, WithProfiles(profiles))
	ctx := context.Background()
	_ = invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})

	u, err := svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "bob@b.com")
	if err != nil {
		t.Fatal(err)
	}

	profs, err := profiles.GetByUserID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profs))
	}
	if !profs[0].IsDefault {
		t.Fatal("expected default profile")
	}
	if profs[0].Name != "bob" {
		t.Fatalf("expected profile name 'bob', got %s", profs[0].Name)
	}

	evts := events.PublishedEvents()
	foundUserCreated := false
	foundProfileCreated := false
	for _, e := range evts {
		switch e.EventName() {
		case "user.created":
			foundUserCreated = true
		case "user.profile.created":
			foundProfileCreated = true
		}
	}
	if !foundUserCreated {
		t.Fatal("expected user.created event")
	}
	if !foundProfileCreated {
		t.Fatal("expected profile.created event with profiles enabled")
	}
}

// Without a UnitOfWork, profile create still fails the whole flow now: the
// apply closure short-circuits on profile error so the user is not "left
// behind" with a missing default profile. (The decrement compensation in the
// no-uow legacy path only covers the invite slot - user persistence is the
// fake's responsibility, not the service's.)
func TestRegisterWithInvite_WithProfiles_ProfileCreateError_NoUOW_Fails(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	quickConnect := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	profiles := ufakes.NewFakeProfileRepository()
	events := fakes.NewFakeEventBus()

	svc := NewAccessService(users, apiKeys, invites, quickConnect, hasher, events, WithProfiles(profiles))
	ctx := context.Background()
	_ = invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
	profiles.Err = errors.New("profile create fail")

	if _, err := svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "bob@b.com"); err == nil {
		t.Fatal("expected error when profile create fails (atomic flow)")
	}
	// No events should have been published - events fire only on commit.
	for _, e := range events.PublishedEvents() {
		if e.EventName() == "user.created" || e.EventName() == "user.profile.created" {
			t.Fatalf("no creation events expected on failure, got %s", e.EventName())
		}
	}
}

// With a UnitOfWork, a failing profile.Create rolls back the whole
// transaction: the user must NOT be persisted, and no creation events must
// be published. This is the core regression test for the partial-write
// TOCTOU that motivated the atomicity refactor.
func TestRegisterWithInvite_WithProfiles_ProfileCreateError_UOW_RollsBack(t *testing.T) {
	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	quickConnect := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	profiles := ufakes.NewFakeProfileRepository()
	events := fakes.NewFakeEventBus()

	uow := &snapshotAccessUOW{
		inner:        fakes.NewFakeTransactionalUnit(),
		users:        users,
		invites:      invites,
		inviteTokens: []string{"rb-tok"},
	}
	svc := NewAccessService(users, apiKeys, invites, quickConnect, hasher, events, WithProfiles(profiles), WithAccessUnitOfWork(uow))
	ctx := context.Background()
	_ = invites.Create(ctx, &user.InviteLink{ID: "inv-rb", Token: "rb-tok", MaxUses: 1})
	profiles.Err = errors.New("profile db down")

	if _, err := svc.RegisterWithInvite(ctx, "rb-tok", "bob", "longpassword", "b@b.com"); err == nil {
		t.Fatal("expected error when profile create fails inside tx")
	}

	// User must have been rolled back.
	all, _ := users.List(ctx)
	if len(all) != 0 {
		t.Fatalf("expected user rolled back, got %d users", len(all))
	}
	// Invite slot must have been rolled back too - a retry on the MaxUses=1
	// invite should still succeed once the underlying error is gone.
	inv, _ := invites.GetByToken(ctx, "rb-tok")
	if inv.UsedCount != 0 {
		t.Fatalf("expected UsedCount rolled back to 0, got %d", inv.UsedCount)
	}
	// No creation events on rollback.
	for _, e := range events.PublishedEvents() {
		if e.EventName() == "user.created" || e.EventName() == "user.profile.created" {
			t.Fatalf("no creation events expected on rollback, got %s", e.EventName())
		}
	}

	// And the retry succeeds - proves slot truly was rolled back.
	profiles.Err = nil
	if u, err := svc.RegisterWithInvite(ctx, "rb-tok", "carol", "longpassword", "c@b.com"); err != nil || u == nil {
		t.Fatalf("retry on rolled-back invite slot should succeed, got u=%v err=%v", u, err)
	}
}

// snapshotAccessUOW wraps FakeTransactionalUnit to undo writes the apply
// closure performed against the in-memory fakes when fn returns an error -
// the fakes don't know about the ambient tx so we simulate rollback here.
// Mirrors snapshotUOW used by user_service_test.go for the same purpose.
type snapshotAccessUOW struct {
	inner        *fakes.FakeTransactionalUnit
	users        *ufakes.FakeUserRepository
	invites      *ufakes.FakeInviteLinkRepository
	inviteTokens []string // tokens whose UsedCount we restore on rollback
}

func (s *snapshotAccessUOW) Do(ctx context.Context, fn func(context.Context) error) error {
	beforeUsers, _ := s.users.List(ctx)
	beforeIDs := map[string]struct{}{}
	for _, u := range beforeUsers {
		beforeIDs[string(u.ID)] = struct{}{}
	}
	// Snapshot used_count keyed by invite token (the fake exposes only
	// GetByToken for lookups; tests supply tokens explicitly).
	beforeUsed := map[string]int{}
	for _, tok := range s.inviteTokens {
		if link, err := s.invites.GetByToken(ctx, tok); err == nil {
			beforeUsed[tok] = link.UsedCount
		}
	}

	err := s.inner.Do(ctx, fn)
	if err == nil {
		return nil
	}
	// Roll back any user inserted by fn.
	after, _ := s.users.List(ctx)
	for _, u := range after {
		if _, ok := beforeIDs[string(u.ID)]; !ok {
			_ = s.users.Delete(ctx, u.ID)
		}
	}
	// Restore each tracked invite's used_count.
	for _, tok := range s.inviteTokens {
		prev, ok := beforeUsed[tok]
		if !ok {
			continue
		}
		link, gerr := s.invites.GetByToken(ctx, tok)
		if gerr != nil {
			continue
		}
		for link.UsedCount > prev {
			if dErr := s.invites.DecrementUsedCount(ctx, link.ID); dErr != nil {
				break
			}
			link, gerr = s.invites.GetByToken(ctx, tok)
			if gerr != nil {
				break
			}
		}
	}
	return err
}

// --- Publish error observability (kills err != nil mutations) ---

// inviteRepoIncrementFail wraps FakeInviteLinkRepository to fail only on IncrementUsedCount.
type inviteRepoIncrementFail struct {
	*ufakes.FakeInviteLinkRepository
	incErr error
}

func (r *inviteRepoIncrementFail) IncrementUsedCount(ctx context.Context, id string) error {
	return r.incErr
}

// When the atomic claim fails (DB error or no-longer-usable), registration
// must fail closed - creating a user without successfully claiming a slot
// would reopen the TOCTOU window fixed in the same change.
func TestRegisterWithInvite_IncrementUsedCountError_FailsClosed(t *testing.T) {
	base := ufakes.NewFakeInviteLinkRepository()
	_ = base.Create(context.Background(), &user.InviteLink{ID: "inv1", Token: "tok"})
	invites := &inviteRepoIncrementFail{FakeInviteLinkRepository: base, incErr: errors.New("increment fail")}

	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	qc := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	events := fakes.NewFakeEventBus()
	svc := NewAccessService(users, apiKeys, invites, qc, hasher, events)

	u, err := svc.RegisterWithInvite(context.Background(), "tok", "bob", "longpassword", "b@b.com")
	if err == nil {
		t.Fatalf("expected error when IncrementUsedCount fails; got user=%v", u)
	}
	if u != nil {
		t.Fatal("expected nil user when claim fails")
	}
}

// userRepoCreateErr wraps FakeUserRepository to inject a transient Create
// error without breaking reads/writes after the first attempt.
type userRepoCreateErrOnce struct {
	*ufakes.FakeUserRepository
	failOnce bool
}

func (r *userRepoCreateErrOnce) Create(ctx context.Context, u *user.User) error {
	if r.failOnce {
		r.failOnce = false
		return errors.New("transient db error")
	}
	return r.FakeUserRepository.Create(ctx, u)
}

// When user.Create fails AFTER the invite slot is claimed, the slot must be
// decremented so a MaxUses=1 invite remains usable for a valid retry.
func TestRegisterWithInvite_UsersCreateFailure_DecrementsSlot(t *testing.T) {
	base := ufakes.NewFakeInviteLinkRepository()
	_ = base.Create(context.Background(), &user.InviteLink{ID: "inv-rb", Token: "tok-rb", MaxUses: 1})

	users := &userRepoCreateErrOnce{FakeUserRepository: ufakes.NewFakeUserRepository(), failOnce: true}
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	qc := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	events := fakes.NewFakeEventBus()
	svc := NewAccessService(users, apiKeys, base, qc, hasher, events)

	_, err := svc.RegisterWithInvite(context.Background(), "tok-rb", "bob", "longpassword", "b@b.com")
	if err == nil {
		t.Fatal("expected error on transient create failure")
	}

	// Slot must have been rolled back. A retry on the same (MaxUses=1) invite
	// should succeed - fail-closed if the slot were still burned.
	u, err := svc.RegisterWithInvite(context.Background(), "tok-rb", "carol", "longpassword", "c@b.com")
	if err != nil {
		t.Fatalf("slot should have been rolled back, got %v", err)
	}
	if u == nil {
		t.Fatal("expected user on retry")
	}
}

func TestRegisterWithInvite_PublishError_EmitsWarning(t *testing.T) {
	read, restore := captureSlog(t)
	defer restore()

	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
	d.events.PublishErr = errors.New("bus down")

	if _, err := d.svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "b@b.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), "publish event") {
		t.Fatalf("expected publish warning, got: %q", read())
	}
}

func TestRegisterWithInvite_WithProfiles_PublishError_EmitsWarning(t *testing.T) {
	read, restore := captureSlog(t)
	defer restore()

	users := ufakes.NewFakeUserRepository()
	apiKeys := ufakes.NewFakeAPIKeyRepository()
	invites := ufakes.NewFakeInviteLinkRepository()
	qc := ufakes.NewFakeQuickConnectRepository()
	hasher := ufakes.NewFakePasswordHasher()
	profiles := ufakes.NewFakeProfileRepository()
	events := fakes.NewFakeEventBus()
	svc := NewAccessService(users, apiKeys, invites, qc, hasher, events, WithProfiles(profiles))

	ctx := context.Background()
	_ = invites.Create(ctx, &user.InviteLink{ID: "inv1", Token: "tok"})
	events.PublishErr = errors.New("bus down")

	u, err := svc.RegisterWithInvite(ctx, "tok", "bob", "longpassword", "b@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil {
		t.Fatal("expected user")
	}
	out := read()
	if strings.Count(out, "publish event") < 2 {
		t.Fatalf("expected >=2 publish warnings (user + profile), got: %q", out)
	}
}

func TestCreateAPIKey_PublishError_EmitsWarning(t *testing.T) {
	read, restore := captureSlog(t)
	defer restore()

	d := newAccessTestDeps()
	d.events.PublishErr = errors.New("bus down")
	if err := d.svc.CreateAPIKey(context.Background(), &user.APIKeyModel{ID: "k1", UserID: "u1", Name: "k", KeyHash: "h", Permissions: []string{"media:read"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), "publish event") {
		t.Fatalf("expected publish warning, got: %q", read())
	}
}

func TestRevokeAPIKey_PublishError_EmitsWarning(t *testing.T) {
	read, restore := captureSlog(t)
	defer restore()

	d := newAccessTestDeps()
	ctx := context.Background()
	_ = d.apiKeys.Create(ctx, &user.APIKeyModel{ID: "k1", UserID: "u1"})
	d.events.PublishErr = errors.New("bus down")
	if err := d.svc.RevokeAPIKey(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(), "publish event") {
		t.Fatalf("expected publish warning, got: %q", read())
	}
}

// The public registration form asks whether a code is still good before
// showing the fields: a live code answers with its admin flag; an unknown,
// expired, exhausted or revoked one answers the SAME neutral "no" with no
// error, so the form gives no oracle on which codes ever existed.
func TestValidateInvite_NeutralOnEveryDeadCode(t *testing.T) {
	d := newAccessTestDeps()
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	for _, l := range []*user.InviteLink{
		{ID: "live", Token: "tok-live", CreatedBy: "u1", MaxUses: 1},
		{ID: "admin", Token: "tok-admin", CreatedBy: "u1", MaxUses: 5, IsAdmin: true},
		{ID: "expired", Token: "tok-expired", CreatedBy: "u1", MaxUses: 1, ExpiresAt: &past},
		{ID: "used", Token: "tok-used", CreatedBy: "u1", MaxUses: 2, UsedCount: 2},
	} {
		if err := d.svc.CreateInviteLink(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	valid, isAdmin, err := d.svc.ValidateInvite(ctx, "tok-live")
	if err != nil || !valid || isAdmin {
		t.Errorf("live member code: valid=%v admin=%v err=%v", valid, isAdmin, err)
	}
	valid, isAdmin, err = d.svc.ValidateInvite(ctx, "tok-admin")
	if err != nil || !valid || !isAdmin {
		t.Errorf("live admin code: valid=%v admin=%v err=%v", valid, isAdmin, err)
	}
	for _, tok := range []string{"tok-expired", "tok-used", "tok-never-existed", ""} {
		valid, isAdmin, err = d.svc.ValidateInvite(ctx, tok)
		if err != nil || valid || isAdmin {
			t.Errorf("%q must be a neutral no: valid=%v admin=%v err=%v", tok, valid, isAdmin, err)
		}
	}
}
