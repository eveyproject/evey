package mediarequest_test

import (
	"context"
	"errors"
	"testing"

	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
)

// --- fakes ---

type fakeRepo struct {
	created []*mediarequest.Request
	updated []*mediarequest.Request
	byID    map[string]*mediarequest.Request
	hasOpen bool
	hasErr  error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byID: map[string]*mediarequest.Request{}} }

func (f *fakeRepo) Create(_ context.Context, r *mediarequest.Request) error {
	f.created = append(f.created, r)
	f.byID[r.ID] = r
	return nil
}
func (f *fakeRepo) GetByID(_ context.Context, id string) (*mediarequest.Request, error) {
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, mediarequest.ErrNotFound
}
func (f *fakeRepo) Update(_ context.Context, r *mediarequest.Request) error {
	f.updated = append(f.updated, r)
	return nil
}
func (f *fakeRepo) Delete(context.Context, string) error { return nil }
func (f *fakeRepo) ListByProfile(context.Context, string, int, int) ([]*mediarequest.Request, error) {
	return nil, nil
}
func (f *fakeRepo) ListAll(context.Context, []mediarequest.Status, int, int) ([]*mediarequest.Request, error) {
	return nil, nil
}
func (f *fakeRepo) HasOpenRequest(context.Context, string, mediarequest.MediaType, string) (bool, error) {
	return f.hasOpen, f.hasErr
}

type fakeAccess struct {
	global    bool
	overrides map[string]bool
}

func (f *fakeAccess) GlobalDefault(context.Context) (bool, error) { return f.global, nil }
func (f *fakeAccess) SetGlobalDefault(_ context.Context, v bool) error {
	f.global = v
	return nil
}
func (f *fakeAccess) Override(_ context.Context, userID string) (bool, bool, error) {
	v, ok := f.overrides[userID]
	return v, ok, nil
}
func (f *fakeAccess) SetOverride(_ context.Context, userID string, v bool) error {
	f.overrides[userID] = v
	return nil
}
func (f *fakeAccess) ClearOverride(_ context.Context, userID string) error {
	delete(f.overrides, userID)
	return nil
}
func (f *fakeAccess) ListOverrides(context.Context) (map[string]bool, error) { return f.overrides, nil }

type fakeBus struct{ published []shared.Event }

func (f *fakeBus) Publish(_ context.Context, e shared.Event) error {
	f.published = append(f.published, e)
	return nil
}
func (f *fakeBus) Subscribe(string, func(context.Context, shared.Event)) string { return "" }
func (f *fakeBus) Unsubscribe(string)                                           {}

func newService(repo *fakeRepo, access *fakeAccess, bus *fakeBus) *appmediarequest.Service {
	return appmediarequest.NewService(repo, access, nil, bus)
}

// --- tests ---

func TestCreateRequest_FailsClosedWhenDisabled(t *testing.T) {
	repo := newFakeRepo()
	bus := &fakeBus{}
	access := &fakeAccess{global: false, overrides: map[string]bool{}}
	svc := newService(repo, access, bus)

	_, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Dune",
	})
	if !errors.Is(err, mediarequest.ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if len(repo.created) != 0 {
		t.Error("request was persisted despite disabled policy")
	}
	if len(bus.published) != 0 {
		t.Error("event was published despite disabled policy")
	}
}

func TestCreateRequest_OverrideBeatsGlobalDefault(t *testing.T) {
	repo := newFakeRepo()
	bus := &fakeBus{}
	// Global default OFF, but this user is explicitly enabled.
	access := &fakeAccess{global: false, overrides: map[string]bool{"usr-1": true}}
	svc := newService(repo, access, bus)

	req, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeTV, Title: "Severance",
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if len(repo.created) != 1 || req.Status != mediarequest.StatusPending {
		t.Error("request not persisted as pending")
	}
	if len(bus.published) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.published))
	}
	if _, ok := bus.published[0].(mediarequest.RequestCreated); !ok {
		t.Errorf("event = %T, want RequestCreated", bus.published[0])
	}
}

func TestCreateRequest_RejectsDuplicate(t *testing.T) {
	repo := newFakeRepo()
	repo.hasOpen = true
	bus := &fakeBus{}
	access := &fakeAccess{global: true, overrides: map[string]bool{}}
	svc := newService(repo, access, bus)

	_, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Dune", Provider: "tmdb", ProviderID: "438631",
	})
	if !errors.Is(err, mediarequest.ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
	if len(repo.created) != 0 {
		t.Error("duplicate request was persisted")
	}
}

// fakeProvider records whether the upstream metadata provider was reached.
// Searching burns the operator's own API key/quota, so a user who is denied
// request access must never get that far.
type fakeProvider struct{ searched int }

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Search(_ context.Context, _ string, _ *int, _ string) ([]metadata.MetadataResult, error) {
	f.searched++
	return nil, nil
}
func (f *fakeProvider) GetByProviderID(_ context.Context, _ string, _ string) (*metadata.MetadataResult, error) {
	return nil, nil
}

func TestSearchTitles_DeniedUserNeverReachesProvider(t *testing.T) {
	prov := &fakeProvider{}
	access := &fakeAccess{global: true, overrides: map[string]bool{"usr-blocked": false}}
	svc := appmediarequest.NewService(newFakeRepo(), access, []metadata.MetadataProvider{prov}, &fakeBus{})

	_, err := svc.SearchTitles(context.Background(), "usr-blocked", mediarequest.MediaTypeMovie, "Dune", nil)
	if !errors.Is(err, mediarequest.ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if prov.searched != 0 {
		t.Errorf("provider was queried %d times for a denied user; the operator's API quota must not be reachable", prov.searched)
	}
}

func TestSearchTitles_AllowedUserReachesProvider(t *testing.T) {
	prov := &fakeProvider{}
	access := &fakeAccess{global: true, overrides: map[string]bool{}}
	svc := appmediarequest.NewService(newFakeRepo(), access, []metadata.MetadataProvider{prov}, &fakeBus{})

	if _, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "Dune", nil); err != nil {
		t.Fatalf("SearchTitles: %v", err)
	}
	if prov.searched != 1 {
		t.Errorf("provider queried %d times, want 1", prov.searched)
	}
}

func TestCanRequest_OverridePrecedence(t *testing.T) {
	access := &fakeAccess{global: true, overrides: map[string]bool{"usr-blocked": false}}
	svc := newService(newFakeRepo(), access, &fakeBus{})

	if ok, _ := svc.CanRequest(context.Background(), "usr-blocked"); ok {
		t.Error("explicit disable override should win over enabled default")
	}
	if ok, _ := svc.CanRequest(context.Background(), "usr-other"); !ok {
		t.Error("user without override should follow the enabled default")
	}
}

func TestAnswer_PersistsAndPublishes(t *testing.T) {
	repo := newFakeRepo()
	bus := &fakeBus{}
	access := &fakeAccess{global: true, overrides: map[string]bool{}}
	svc := newService(repo, access, bus)

	created, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Dune",
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	bus.published = nil // drop the created event

	answered, err := svc.Answer(context.Background(), created.ID, mediarequest.StatusApproved, "queued", "prf-admin")
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if answered.Status != mediarequest.StatusApproved || answered.AdminNote != "queued" {
		t.Error("answer not applied")
	}
	if len(repo.updated) != 1 {
		t.Error("update not persisted")
	}
	if len(bus.published) != 1 {
		t.Fatalf("want 1 event, got %d", len(bus.published))
	}
	if _, ok := bus.published[0].(mediarequest.RequestAnswered); !ok {
		t.Errorf("event = %T, want RequestAnswered", bus.published[0])
	}
}

func TestAnswer_RejectsNonAnswerStatus(t *testing.T) {
	svc := newService(newFakeRepo(), &fakeAccess{global: true, overrides: map[string]bool{}}, &fakeBus{})
	if _, err := svc.Answer(context.Background(), "req-x", mediarequest.StatusPending, "", "prf-admin"); err == nil {
		t.Error("expected error answering with a non-answer status")
	}
}
