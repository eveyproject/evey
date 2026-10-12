package mediarequest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	appmediarequest "github.com/eveyproject/evey/internal/app/mediarequest"
	"github.com/eveyproject/evey/internal/domain/mediarequest"
	mrfakes "github.com/eveyproject/evey/internal/domain/mediarequest/fakes"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
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
		MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie",
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
		MediaType: mediarequest.MediaTypeTV, Title: "Some Series",
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

// The poster URL is CLIENT-supplied and later rendered on the admin's
// device: only the artwork CDNs our own metadata providers emit survive;
// anything else is dropped (the request itself still goes through - the
// poster is cosmetic).
func TestCreateRequest_PosterURLHostAllowlist(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://image.tmdb.org/t/p/w500/x.jpg", "https://image.tmdb.org/t/p/w500/x.jpg"},
		{"https://coverartarchive.org/release-group/abc/front-250", "https://coverartarchive.org/release-group/abc/front-250"},
		{"https://evil.example.com/tracker.png", ""},
		{"http://image.tmdb.org/t/p/w500/x.jpg", ""}, // https only
		{"javascript:alert(1)", ""},
		{"", ""},
	}
	for _, c := range cases {
		repo := mrfakes.NewFakeRepository()
		svc := newService(repo, mrfakes.NewFakeAccess(true), &fakeBus{})
		req, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
			MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie", PosterURL: c.in,
		})
		if err != nil {
			t.Fatalf("CreateRequest(%q): %v", c.in, err)
		}
		if req.PosterURL != c.want {
			t.Errorf("PosterURL(%q) = %q, want %q", c.in, req.PosterURL, c.want)
		}
	}
}

func TestCreateRequest_RejectsDuplicate(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	repo.HasOpen = true
	bus := &fakeBus{}
	access := mrfakes.NewFakeAccess(true)
	svc := newService(repo, access, bus)

	_, err := svc.CreateRequest(context.Background(), "prf-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie", Provider: "tmdb", ProviderID: "438631",
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

	_, err := svc.SearchTitles(context.Background(), "usr-blocked", mediarequest.MediaTypeMovie, "Some Movie", nil)
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

	if _, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "Some Movie", nil); err != nil {
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
		MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie",
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

// countingLookup records whether the library was consulted at all - which is
// what the three guards in markInLibrary decide.
type countingLookup struct {
	calls int
	held  map[string]string
}

func (c *countingLookup) FindByProviderIDs(_ context.Context, _ string, _ []string) (map[string]string, error) {
	c.calls++
	return c.held, nil
}

type resultProvider struct{ results []metadata.MetadataResult }

func (p *resultProvider) Name() string { return "results" }
func (p *resultProvider) Search(_ context.Context, _ string, _ *int, _ string) ([]metadata.MetadataResult, error) {
	return p.results, nil
}
func (p *resultProvider) GetByProviderID(_ context.Context, _ string, _ string) (*metadata.MetadataResult, error) {
	return nil, nil
}

// markInLibrary is gated by three independent conditions and mutation testing
// showed each one could be negated with the suite still green. The music term in
// particular is a documented product decision - music_tracks carries no
// MusicBrainz id, so the only join would be on album title, and a wrong "you
// already have this" is worse than showing no badge at all.
func TestSearchTitles_LibraryLookupGuards(t *testing.T) {
	results := []metadata.MetadataResult{{Title: "Some Movie", ProviderIDs: map[string]string{"results": "123"}}}

	t.Run("music never reaches the library lookup", func(t *testing.T) {
		lookup := &countingLookup{}
		svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true),
			[]metadata.MetadataProvider{&resultProvider{results: results}}, &fakeBus{}).WithLibraryLookup(lookup)

		if _, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMusic, "Some Album", nil); err != nil {
			t.Fatalf("SearchTitles: %v", err)
		}
		if lookup.calls != 0 {
			t.Errorf("the library was consulted %d times for music; a wrong in-library badge is worse than none", lookup.calls)
		}
	})

	t.Run("a movie search does reach it", func(t *testing.T) {
		lookup := &countingLookup{held: map[string]string{}}
		svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true),
			[]metadata.MetadataProvider{&resultProvider{results: results}}, &fakeBus{}).WithLibraryLookup(lookup)

		if _, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "Some Movie", nil); err != nil {
			t.Fatalf("SearchTitles: %v", err)
		}
		if lookup.calls != 1 {
			t.Errorf("library consulted %d times for a movie search, want 1", lookup.calls)
		}
	})

	t.Run("no candidates means no lookup", func(t *testing.T) {
		lookup := &countingLookup{}
		svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true),
			[]metadata.MetadataProvider{&resultProvider{results: nil}}, &fakeBus{}).WithLibraryLookup(lookup)

		if _, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "Some Movie", nil); err != nil {
			t.Fatalf("SearchTitles: %v", err)
		}
		if lookup.calls != 0 {
			t.Errorf("library consulted %d times with no candidates, want 0", lookup.calls)
		}
	})

	t.Run("an unwired library is simply skipped", func(t *testing.T) {
		// No WithLibraryLookup: the search must still succeed, unflagged.
		svc := appmediarequest.NewService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true),
			[]metadata.MetadataProvider{&resultProvider{results: results}}, &fakeBus{})
		out, err := svc.SearchTitles(context.Background(), "usr-1", mediarequest.MediaTypeMovie, "Some Movie", nil)
		if err != nil {
			t.Fatalf("SearchTitles without a library lookup must still work: %v", err)
		}
		if len(out) != 1 || out[0].InLibrary {
			t.Errorf("candidates must come back unflagged, got %+v", out)
		}
	})
}

// failingBus rejects every publish, standing in for a saturated or broken bus.
type failingBus struct{}

func (failingBus) Publish(context.Context, shared.Event) error {
	return errors.New("bus unavailable")
}
func (failingBus) Subscribe(string, func(context.Context, shared.Event)) string { return "" }
func (failingBus) Unsubscribe(string)                                           {}

// Telemetry must never decide whether the user's action succeeded: the request
// is already persisted by the time the event is published. Both publish sites
// survived mutation, meaning nothing asserted that contract.
func TestCreateAndAnswer_SurviveAFailingEventBus(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	svc := appmediarequest.NewService(repo, mrfakes.NewFakeAccess(true), nil, failingBus{})

	req, err := svc.CreateRequest(context.Background(), "prof-1", "usr-1", appmediarequest.CreateInput{
		MediaType: mediarequest.MediaTypeMovie, Title: "Some Movie",
	})
	if err != nil {
		t.Fatalf("a failing event bus must not fail the request: %v", err)
	}
	if req == nil {
		t.Fatal("the request must still be returned")
	}

	if _, err := svc.Answer(context.Background(), req.ID, mediarequest.StatusApproved, "ok", "prof-admin"); err != nil {
		t.Fatalf("a failing event bus must not fail the answer: %v", err)
	}
}

// A page size is the client's to ask for, within a ceiling: an absent or
// oversized limit falls back to the default instead of loading the whole table.
func TestList_PageSizeIsClamped(t *testing.T) {
	repo := mrfakes.NewFakeRepository()
	ctx := context.Background()
	for i := range 600 {
		r, err := mediarequest.NewRequest(fmt.Sprintf("req-%03d", i), "prf-1", "usr-1", mediarequest.MediaTypeMovie, "Some Movie")
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Create(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	svc := newService(repo, mrfakes.NewFakeAccess(true), &fakeBus{})

	for _, c := range []struct{ limit, want int }{{0, 50}, {200, 200}, {201, 50}} {
		got, err := svc.ListMyRequests(ctx, "prf-1", c.limit, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("ListMyRequests(limit=%d) = %d rows, want %d", c.limit, len(got), c.want)
		}
	}
	for _, c := range []struct{ limit, want int }{{0, 100}, {500, 500}, {501, 100}} {
		got, err := svc.ListAll(ctx, nil, c.limit, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("ListAll(limit=%d) = %d rows, want %d", c.limit, len(got), c.want)
		}
	}
}
