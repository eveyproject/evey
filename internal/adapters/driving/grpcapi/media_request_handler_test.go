package grpcapi

import (
	"context"
	"sort"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appmediarequest "github.com/eveyproject/evey/internal/app/mediarequest"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/mediarequest"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- fakes ---
//
// mediarequest.Repository/Access have no fakes package yet, and the app-level
// fakeRepo in internal/app/mediarequest/service_test.go doesn't filter
// (ListByProfile/ListAll both return nil, nil) - it can't exercise the
// handler's ownership scoping. These local fakes are real in-memory state
// machines (not mocks with expectations), matching the package's style
// (see libfakes.FakeFavoriteRepository used throughout favorite_handler_test.go).

type fakeMediaRequestRepo struct {
	byID map[string]*mediarequest.Request
}

func newFakeMediaRequestRepo() *fakeMediaRequestRepo {
	return &fakeMediaRequestRepo{byID: map[string]*mediarequest.Request{}}
}

func (f *fakeMediaRequestRepo) seed(r *mediarequest.Request) *mediarequest.Request {
	f.byID[r.ID] = r
	return r
}

func (f *fakeMediaRequestRepo) Create(_ context.Context, r *mediarequest.Request) error {
	f.byID[r.ID] = r
	return nil
}

func (f *fakeMediaRequestRepo) GetByID(_ context.Context, id string) (*mediarequest.Request, error) {
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, mediarequest.ErrNotFound
}

func (f *fakeMediaRequestRepo) Update(_ context.Context, r *mediarequest.Request) error {
	f.byID[r.ID] = r
	return nil
}

func (f *fakeMediaRequestRepo) Delete(_ context.Context, id string) error {
	delete(f.byID, id)
	return nil
}

func (f *fakeMediaRequestRepo) ListByProfile(_ context.Context, profileID string, limit, offset int) ([]*mediarequest.Request, error) {
	var out []*mediarequest.Request
	for _, r := range f.byID {
		if string(r.ProfileID) == profileID {
			out = append(out, r)
		}
	}
	sortRequestsNewestFirst(out)
	return paginateRequests(out, limit, offset), nil
}

func (f *fakeMediaRequestRepo) ListAll(_ context.Context, statuses []mediarequest.Status, limit, offset int) ([]*mediarequest.Request, error) {
	want := make(map[mediarequest.Status]bool, len(statuses))
	for _, s := range statuses {
		want[s] = true
	}
	var out []*mediarequest.Request
	for _, r := range f.byID {
		if len(want) == 0 || want[r.Status] {
			out = append(out, r)
		}
	}
	sortRequestsNewestFirst(out)
	return paginateRequests(out, limit, offset), nil
}

func (f *fakeMediaRequestRepo) HasOpenRequest(_ context.Context, profileID string, mediaType mediarequest.MediaType, providerID string) (bool, error) {
	for _, r := range f.byID {
		if string(r.ProfileID) == profileID && r.MediaType == mediaType && r.ProviderID == providerID && r.Status != mediarequest.StatusRejected {
			return true, nil
		}
	}
	return false, nil
}

func sortRequestsNewestFirst(rs []*mediarequest.Request) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].CreatedAt.After(rs[j].CreatedAt) })
}

func paginateRequests(items []*mediarequest.Request, limit, offset int) []*mediarequest.Request {
	if offset >= len(items) {
		return nil
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

type fakeMediaRequestAccess struct {
	global    bool
	overrides map[string]bool
}

func newFakeMediaRequestAccess(global bool) *fakeMediaRequestAccess {
	return &fakeMediaRequestAccess{global: global, overrides: map[string]bool{}}
}

func (f *fakeMediaRequestAccess) GlobalDefault(context.Context) (bool, error) { return f.global, nil }

func (f *fakeMediaRequestAccess) SetGlobalDefault(_ context.Context, v bool) error {
	f.global = v
	return nil
}

func (f *fakeMediaRequestAccess) Override(_ context.Context, userID string) (bool, bool, error) {
	v, ok := f.overrides[userID]
	return v, ok, nil
}

func (f *fakeMediaRequestAccess) SetOverride(_ context.Context, userID string, v bool) error {
	f.overrides[userID] = v
	return nil
}

func (f *fakeMediaRequestAccess) ClearOverride(_ context.Context, userID string) error {
	delete(f.overrides, userID)
	return nil
}

func (f *fakeMediaRequestAccess) ListOverrides(context.Context) (map[string]bool, error) {
	return f.overrides, nil
}

// newTestMediaRequestHandlers builds both the requester-facing and the admin
// handler on top of the same service/fakes, mirroring how they're wired in
// production (one *appmediarequest.Service shared by both gRPC surfaces).
func newTestMediaRequestHandlers(access *fakeMediaRequestAccess) (*mediaRequestHandler, *adminMediaRequestHandler, *fakeMediaRequestRepo) {
	repo := newFakeMediaRequestRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := appmediarequest.NewService(repo, access, nil, bus)
	return &mediaRequestHandler{svc: svc}, &adminMediaRequestHandler{svc: svc}, repo
}

// --- toMediaRequestProto ---

func TestToMediaRequestProto_FullMapping(t *testing.T) {
	created := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)
	answered := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	year := 2021

	r := &mediarequest.Request{
		ID:         "req-1",
		ProfileID:  "profile-1",
		UserID:     "user-1",
		MediaType:  mediarequest.MediaTypeMovie,
		Provider:   "tmdb",
		ProviderID: "438631",
		Title:      "Some Movie",
		Year:       &year,
		PosterURL:  "https://example.com/poster.jpg",
		Status:     mediarequest.StatusApproved,
		AdminNote:  "queued for next batch",
		AnsweredBy: "profile-admin",
		CreatedAt:  created,
		UpdatedAt:  updated,
		AnsweredAt: &answered,
	}

	p := toMediaRequestProto(r, "Alice")

	assert.Equal(t, "req-1", p.Id)
	assert.Equal(t, "profile-1", p.ProfileId)
	assert.Equal(t, "user-1", p.UserId)
	assert.Equal(t, string(mediarequest.MediaTypeMovie), p.MediaType)
	assert.Equal(t, "tmdb", p.Provider)
	assert.Equal(t, "438631", p.ProviderId)
	assert.Equal(t, "Some Movie", p.Title)
	assert.Equal(t, int32(2021), p.Year)
	assert.Equal(t, "https://example.com/poster.jpg", p.PosterUrl)
	assert.Equal(t, string(mediarequest.StatusApproved), p.Status)
	assert.Equal(t, "queued for next batch", p.AdminNote)
	assert.Equal(t, "profile-admin", p.AnsweredBy)
	assert.Equal(t, "Alice", p.RequesterName)
	require.NotNil(t, p.CreatedAt)
	assert.Equal(t, created.Unix(), p.CreatedAt.AsTime().Unix())
	require.NotNil(t, p.UpdatedAt)
	assert.Equal(t, updated.Unix(), p.UpdatedAt.AsTime().Unix())
	require.NotNil(t, p.AnsweredAt)
	assert.Equal(t, answered.Unix(), p.AnsweredAt.AsTime().Unix())
}

// The pending/requester-view case: no year, no answer yet, and the empty
// requesterName the non-admin surface always passes.
func TestToMediaRequestProto_PendingUnanswered(t *testing.T) {
	created := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)

	r := &mediarequest.Request{
		ID:        "req-2",
		ProfileID: "profile-2",
		UserID:    "user-2",
		MediaType: mediarequest.MediaTypeTV,
		Title:     "Some Series",
		Status:    mediarequest.StatusPending,
		CreatedAt: created,
		UpdatedAt: created,
	}

	p := toMediaRequestProto(r, "")

	assert.Equal(t, "req-2", p.Id)
	assert.Equal(t, string(mediarequest.StatusPending), p.Status)
	assert.Equal(t, int32(0), p.Year, "no Year set on the domain request must map to the zero value, not panic on nil deref")
	assert.Empty(t, p.RequesterName)
	assert.Empty(t, p.AdminNote)
	assert.Empty(t, p.AnsweredBy)
	assert.Nil(t, p.AnsweredAt, "unanswered request must not carry an AnsweredAt timestamp")
}

// --- ListMyRequests ownership scoping ---

// A requester must only ever see their OWN requests - never another
// profile's, even though the fake repo holds both.
func TestMediaRequestHandler_ListMyRequests_ScopedToOwnProfile(t *testing.T) {
	h, _, repo := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))

	now := time.Now()
	repo.seed(&mediarequest.Request{
		ID: "req-mine-1", ProfileID: "profile-1", UserID: "user-1",
		MediaType: mediarequest.MediaTypeMovie, Title: "Mine 1", Status: mediarequest.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	repo.seed(&mediarequest.Request{
		ID: "req-mine-2", ProfileID: "profile-1", UserID: "user-1",
		MediaType: mediarequest.MediaTypeMovie, Title: "Mine 2", Status: mediarequest.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	repo.seed(&mediarequest.Request{
		ID: "req-stranger", ProfileID: "profile-stranger", UserID: "user-stranger",
		MediaType: mediarequest.MediaTypeMovie, Title: "Not mine", Status: mediarequest.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ListMyRequests(ctx, &eveyv1.ListMyRequestsRequest{})
	require.NoError(t, err)

	require.Len(t, resp.Items, 2, "must return exactly the caller's own requests")
	var ids []string
	for _, it := range resp.Items {
		ids = append(ids, it.Id)
		assert.Equal(t, "profile-1", it.ProfileId, "leaked a request belonging to another profile")
	}
	assert.ElementsMatch(t, []string{"req-mine-1", "req-mine-2"}, ids)
}

func TestMediaRequestHandler_ListMyRequests_Unauthenticated(t *testing.T) {
	h, _, _ := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))

	_, err := h.ListMyRequests(context.Background(), &eveyv1.ListMyRequestsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// --- admin RBAC-sensitive surface (RBAC gating itself lives in the
// interceptor / MethodRoles, ahead of these handlers - these just prove the
// happy path and its response mapping once an admin request reaches here) ---

func TestAdminMediaRequestHandler_ListRequests_HappyPath(t *testing.T) {
	_, admin, repo := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))

	now := time.Now()
	repo.seed(&mediarequest.Request{
		ID: "req-a", ProfileID: "profile-1", UserID: "user-1",
		MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie", Status: mediarequest.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})
	repo.seed(&mediarequest.Request{
		ID: "req-b", ProfileID: "profile-2", UserID: "user-2",
		MediaType: mediarequest.MediaTypeTV, Title: "Some Series", Status: mediarequest.StatusApproved,
		CreatedAt: now, UpdatedAt: now,
	})

	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	resp, err := admin.ListRequests(ctx, &eveyv1.AdminListRequestsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)

	byID := make(map[string]*eveyv1.MediaRequestProto, len(resp.Items))
	for _, it := range resp.Items {
		byID[it.Id] = it
	}
	require.Contains(t, byID, "req-a")
	assert.Equal(t, "Some Movie", byID["req-a"].Title)
	assert.Equal(t, string(mediarequest.StatusPending), byID["req-a"].Status)
	require.Contains(t, byID, "req-b")
	assert.Equal(t, string(mediarequest.StatusApproved), byID["req-b"].Status)
}

func TestAdminMediaRequestHandler_AnswerRequest_HappyPath(t *testing.T) {
	requester, admin, _ := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))

	requesterCtx := ctxWithClaims("user-1", "profile-1", false)
	created, err := requester.CreateRequest(requesterCtx, &eveyv1.CreateRequestRequest{
		MediaType: string(mediarequest.MediaTypeMovie), Title: "Some Movie",
	})
	require.NoError(t, err)
	assert.Equal(t, string(mediarequest.StatusPending), created.Status)

	adminCtx := ctxWithClaims("admin-1", "profile-admin", true)
	answered, err := admin.AnswerRequest(adminCtx, &eveyv1.AnswerRequestRequest{
		Id:     created.Id,
		Status: string(mediarequest.StatusApproved),
		Note:   "queued",
	})
	require.NoError(t, err)
	assert.Equal(t, string(mediarequest.StatusApproved), answered.Status)
	assert.Equal(t, "queued", answered.AdminNote)
	assert.Equal(t, "profile-admin", answered.AnsweredBy, "must record the answering ADMIN's profile, not the requester's")
	require.NotNil(t, answered.AnsweredAt)
}

func TestAdminMediaRequestHandler_AnswerRequest_RejectsNonAnswerStatus(t *testing.T) {
	_, admin, repo := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))
	now := time.Now()
	repo.seed(&mediarequest.Request{
		ID: "req-x", ProfileID: "profile-1", UserID: "user-1",
		MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie", Status: mediarequest.StatusPending,
		CreatedAt: now, UpdatedAt: now,
	})

	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	_, err := admin.AnswerRequest(ctx, &eveyv1.AnswerRequestRequest{Id: "req-x", Status: string(mediarequest.StatusPending)})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestAdminMediaRequestHandler_GetAccessSettings_HappyPath(t *testing.T) {
	access := newFakeMediaRequestAccess(false)
	access.overrides["user-allowed"] = true
	access.overrides["user-blocked"] = false
	_, admin, _ := newTestMediaRequestHandlers(access)

	ctx := ctxWithClaims("admin-1", "profile-admin", true)
	resp, err := admin.GetAccessSettings(ctx, &eveyv1.GetAccessSettingsRequest{})
	require.NoError(t, err)

	assert.False(t, resp.DefaultEnabled)
	got := make(map[string]bool, len(resp.Overrides))
	for _, ov := range resp.Overrides {
		got[ov.UserId] = ov.Enabled
	}
	assert.Equal(t, map[string]bool{"user-allowed": true, "user-blocked": false}, got)
}

func TestAdminMediaRequestHandler_SetUserAccess_HappyPath(t *testing.T) {
	access := newFakeMediaRequestAccess(false)
	_, admin, _ := newTestMediaRequestHandlers(access)
	ctx := ctxWithClaims("admin-1", "profile-admin", true)

	_, err := admin.SetUserAccess(ctx, &eveyv1.SetUserAccessRequest{
		UserId: "user-1", Mode: eveyv1.AccessMode_ACCESS_MODE_ENABLED,
	})
	require.NoError(t, err)
	v, ok := access.overrides["user-1"]
	require.True(t, ok, "override must be recorded")
	assert.True(t, v)

	// ACCESS_MODE_DEFAULT clears the override, falling back to the global default.
	_, err = admin.SetUserAccess(ctx, &eveyv1.SetUserAccessRequest{
		UserId: "user-1", Mode: eveyv1.AccessMode_ACCESS_MODE_DEFAULT,
	})
	require.NoError(t, err)
	_, ok = access.overrides["user-1"]
	assert.False(t, ok, "ACCESS_MODE_DEFAULT must clear the override")
}

func TestAdminMediaRequestHandler_SetUserAccess_RequiresUserID(t *testing.T) {
	_, admin, _ := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))
	ctx := ctxWithClaims("admin-1", "profile-admin", true)

	_, err := admin.SetUserAccess(ctx, &eveyv1.SetUserAccessRequest{Mode: eveyv1.AccessMode_ACCESS_MODE_ENABLED})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// The moderation queue labels each request with the requester's name, looked
// up once per account; a deleted account leaves the name blank without
// failing the listing, and an unwired user port simply labels nothing.
func TestAdminMediaRequestHandler_ListRequests_RequesterNames(t *testing.T) {
	_, admin, repo := newTestMediaRequestHandlers(newFakeMediaRequestAccess(true))
	now := time.Now()
	for _, r := range []*mediarequest.Request{
		{ID: "rn-1", ProfileID: "profile-1", UserID: "user-alice", MediaType: mediarequest.MediaTypeMovie, Title: "A", Status: mediarequest.StatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "rn-2", ProfileID: "profile-1", UserID: "user-alice", MediaType: mediarequest.MediaTypeMovie, Title: "B", Status: mediarequest.StatusPending, CreatedAt: now, UpdatedAt: now},
		{ID: "rn-3", ProfileID: "profile-9", UserID: "user-gone", MediaType: mediarequest.MediaTypeMovie, Title: "C", Status: mediarequest.StatusPending, CreatedAt: now, UpdatedAt: now},
	} {
		repo.seed(r)
	}
	userRepo := userfakes.NewFakeUserRepository()
	require.NoError(t, userRepo.Create(context.Background(), &user.User{ID: "user-alice", Username: "alice"}))
	admin.userQuery = appuser.NewQueryService(userRepo, userfakes.NewFakeProfileRepository(), userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository())

	resp, err := admin.ListRequests(ctxWithClaims("admin-1", "profile-admin", true), &eveyv1.AdminListRequestsRequest{})
	require.NoError(t, err)
	names := map[string]string{}
	for _, it := range resp.Items {
		names[it.Id] = it.RequesterName
	}
	assert.Equal(t, map[string]string{"rn-1": "alice", "rn-2": "alice", "rn-3": ""}, names)

	admin.userQuery = nil
	resp, err = admin.ListRequests(ctxWithClaims("admin-1", "profile-admin", true), &eveyv1.AdminListRequestsRequest{})
	require.NoError(t, err)
	for _, it := range resp.Items {
		assert.Empty(t, it.RequesterName)
	}
}
