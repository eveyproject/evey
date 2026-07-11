package metadata

import (
	"context"
	"errors"
	"testing"

	domainlib "github.com/evey-project/evey/internal/domain/library"
)

type fakePersonFetcher struct {
	byName    map[string]*domainlib.PersonDetailsResult
	errByName map[string]error
}

func (f *fakePersonFetcher) GetPersonDetails(_ context.Context, name string) (*domainlib.PersonDetailsResult, error) {
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

var _ = errors.New // keep errors import for future error-path tests
