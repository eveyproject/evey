package mediarequest_test

import (
	"context"
	"errors"
	"testing"

	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	mrfakes "github.com/evey-project/evey/internal/domain/mediarequest/fakes"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
)

// --- fakes ---

type fakeBus struct{ published []shared.Event }

func (f *fakeBus) Publish(_ context.Context, e shared.Event) error {
	f.published = append(f.published, e)
	return nil
}
func (f *fakeBus) Subscribe(string, func(context.Context, shared.Event)) string { return "" }
func (f *fakeBus) Unsubscribe(string)                                           {}

// withOverride seeds one explicit per-user policy override inline.
func withOverride(a *mrfakes.FakeAccess, userID string, v bool) *mrfakes.FakeAccess {
	a.Overrides[userID] = v
	return a
}

func newService(repo *mrfakes.FakeRepository, access *mrfakes.FakeAccess, bus *fakeBus) *appmediarequest.Service {
	return appmediarequest.NewService(repo, access, nil, bus)
}

// --- tests ---

func TestCreateRequest_FailsClosedWhenDisabled(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	bus := &fakeBus{}
	access := mrfakes.NewFakeAccess(false)
	svc := newService(repo, access, bus)

	_, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Dune",
	})
	if !errors.Is(err, mediarequest.ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
	if len(repo.Created) != 0 {
		t.Error("request was persisted despite disabled policy")
	}
	if len(bus.published) != 0 {
		t.Error("event was published despite disabled policy")
	}
}

func TestCreateRequest_OverrideBeatsGlobalDefault(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	bus := &fakeBus{}
	// Global default OFF, but this user is explicitly enabled.
	access := withOverride(mrfakes.NewFakeAccess(false), "usr-1", true)
	svc := newService(repo, access, bus)

	req, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeTV, Title: "Severance",
	})
	if err != nil {
		t.Fatalf("CreateRequest: %v", err)
	}
	if len(repo.Created) != 1 || req.Status != mediarequest.StatusPending {
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
	repo := mrfakes.NewFakeRepository()
	repo.HasOpen = true
	bus := &fakeBus{}
	access := mrfakes.NewFakeAccess(true)
	svc := newService(repo, access, bus)

	_, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Dune", Provider: "tmdb", ProviderID: "438631",
	})
	if !errors.Is(err, mediarequest.ErrDuplicate) {
		t.Fatalf("err = %v, want ErrDuplicate", err)
	}
	if len(repo.Created) != 0 {
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
	access := withOverride(mrfakes.NewFakeAccess(true), "usr-blocked", false)
	svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), access, []metadata.MetadataProvider{prov}, &fakeBus{})

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
	access := mrfakes.NewFakeAccess(true)
	svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), access, []metadata.MetadataProvider{prov}, &fakeBus{})

	if _, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "Dune", nil); err != nil {
		t.Fatalf("SearchTitles: %v", err)
	}
	if prov.searched != 1 {
		t.Errorf("provider queried %d times, want 1", prov.searched)
	}
}

func TestCanRequest_OverridePrecedence(t *testing.T) {
	access := withOverride(mrfakes.NewFakeAccess(true), "usr-blocked", false)
	svc := newService(mrfakes.NewFakeRepository(), access, &fakeBus{})

	if ok, _ := svc.CanRequest(context.Background(), "usr-blocked"); ok {
		t.Error("explicit disable override should win over enabled default")
	}
	if ok, _ := svc.CanRequest(context.Background(), "usr-other"); !ok {
		t.Error("user without override should follow the enabled default")
	}
}

func TestAnswer_PersistsAndPublishes(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	bus := &fakeBus{}
	access := mrfakes.NewFakeAccess(true)
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
	if len(repo.Updated) != 1 {
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
	svc := newService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true), &fakeBus{})
	if _, err := svc.Answer(context.Background(), "req-x", mediarequest.StatusPending, "", "prf-admin"); err == nil {
		t.Error("expected error answering with a non-answer status")
	}
}
