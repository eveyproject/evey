package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
)

type fakePersonFetcher struct {
	byName    map[string]*domainlib.PersonDetailsResult
	errByName map[string]error
	calls     int
}

func (f *fakePersonFetcher) GetPersonDetails(_ context.Context, name string) (*domainlib.PersonDetailsResult, error) {
	f.calls++
	if f.errByName != nil {
		if err, ok := f.errByName[name]; ok {
			return nil, err
		}
	}
	return f.byName[name], nil
}

func TestEnricher_EnrichAllPeopleBios_NoFetcherConfigured(t *testing.T) {
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	n, err := e.EnrichAllPeopleBios(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when fetcher not configured")
	}
	if n != 0 {
		t.Errorf("enriched = %d, want 0", n)
	}
}

func TestEnricher_EnrichAllPeopleBios_EmptyList(t *testing.T) {
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	EnricherWithPersonFetcher(&fakePersonFetcher{})(e)

	n, err := e.EnrichAllPeopleBios(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Errorf("enriched = %d, want 0 (no people)", n)
	}
}

func TestEnricher_EnrichAllPeopleBios_SkipsExistingBio(t *testing.T) {
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	fetcher := &fakePersonFetcher{byName: map[string]*domainlib.PersonDetailsResult{
		"John Doe": {Biography: "should-not-apply"},
	}}
	EnricherWithPersonFetcher(fetcher)(e)

	_ = e.persons.Create(context.Background(), &domainlib.Person{
		ID: "p-1", Name: "John Doe", Biography: "existing bio",
	})

	n, err := e.EnrichAllPeopleBios(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Errorf("enriched = %d, want 0 (existing bio preserved)", n)
	}
	got, _ := e.persons.GetByID(context.Background(), "p-1")
	if got.Biography != "existing bio" {
		t.Errorf("Biography = %q, want preserved", got.Biography)
	}
}

func TestEnricher_EnrichAllPeopleBios_CtxCancelled(t *testing.T) {
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	EnricherWithPersonFetcher(&fakePersonFetcher{})(e)

	_ = e.persons.Create(context.Background(), &domainlib.Person{
		ID: "p-1", Name: "Jane", Biography: "",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately - loop should break on first iteration
	n, err := e.EnrichAllPeopleBios(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Errorf("enriched = %d, want 0 (ctx cancelled)", n)
	}
}

// The bulk backfill honours the same negative cache as the per-item
// enrichment: a person the provider had no biography for is not asked again
// before bioRecheckTTL, and a fresh "no bio" answer is remembered so neither
// this pass nor tomorrow's daily re-enrichment repeats the round-trip.
func TestEnricher_EnrichAllPeopleBios_SkipsRecentNegativeCheck(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	EnricherWithClock(func() time.Time { return now })(e)
	fetcher := &fakePersonFetcher{byName: map[string]*domainlib.PersonDetailsResult{"Jane": {Biography: "late bio"}}}
	EnricherWithPersonFetcher(fetcher)(e)
	checked := now.Add(-24 * time.Hour)
	_ = e.persons.Create(context.Background(), &domainlib.Person{ID: "p-1", Name: "Jane", BioCheckedAt: &checked})

	n, err := e.EnrichAllPeopleBios(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 || fetcher.calls != 0 {
		t.Fatalf("enriched=%d calls=%d, want 0/0: a day-old negative check must not be repeated", n, fetcher.calls)
	}
}

func TestEnricher_EnrichAllPeopleBios_StampsANoBioAnswer(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	EnricherWithClock(func() time.Time { return now })(e)
	EnricherWithPersonFetcher(&fakePersonFetcher{})(e) // answers nil: the provider knows no bio
	_ = e.persons.Create(context.Background(), &domainlib.Person{ID: "p-1", Name: "Jane"})

	n, err := e.EnrichAllPeopleBios(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("enriched = %d, want 0", n)
	}
	got, _ := e.persons.GetByID(context.Background(), "p-1")
	if got.BioCheckedAt == nil || !got.BioCheckedAt.Equal(now) {
		t.Fatalf("BioCheckedAt = %v, want the clock's now: a negative answer must be remembered", got.BioCheckedAt)
	}
}

func TestEnricher_EnrichAllPeopleBios_ProviderErrorLeavesNoStamp(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	e, _ := newTestEnricherMulti(&fakeProvider{name: "tmdb"}, nil)
	EnricherWithClock(func() time.Time { return now })(e)
	EnricherWithPersonFetcher(&fakePersonFetcher{errByName: map[string]error{"Jane": errors.New("429 too many requests")}})(e)
	_ = e.persons.Create(context.Background(), &domainlib.Person{ID: "p-1", Name: "Jane"})

	if _, err := e.EnrichAllPeopleBios(context.Background(), nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got, _ := e.persons.GetByID(context.Background(), "p-1")
	if got.BioCheckedAt != nil {
		t.Fatalf("BioCheckedAt = %v, want nil: a transient failure must not silence the person for 30 days", got.BioCheckedAt)
	}
}

func TestSleepCtx_ReturnsWhenTheTimerFires(t *testing.T) {
	start := time.Now()
	sleepCtx(context.Background(), 20*time.Millisecond)
	if time.Since(start) < 20*time.Millisecond {
		t.Fatal("returned before the pause elapsed")
	}
}

func TestSleepCtx_ReturnsAtOnceWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	sleepCtx(ctx, time.Second)
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("a cancelled context must cut the pause short")
	}
}
