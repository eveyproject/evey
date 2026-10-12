package metadata

import (
	"context"
	"testing"

	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

type chainTestProvider struct {
	name string
}

func (p chainTestProvider) Name() string { return p.name }
func (p chainTestProvider) Search(_ context.Context, _ string, _ *int, _ string) ([]domainmetadata.MetadataResult, error) {
	return nil, nil
}
func (p chainTestProvider) GetByProviderID(_ context.Context, _ string, _ string) (*domainmetadata.MetadataResult, error) {
	return nil, nil
}

type translationCapable struct {
	chainTestProvider
}

func (t translationCapable) FetchTranslations(_ context.Context, _ string, _ string, _ []string) (map[string]domainmetadata.MetadataResult, error) {
	return nil, nil
}
func (t translationCapable) FetchPersonTranslations(_ context.Context, _ string, _ []string) (map[string]string, error) {
	return nil, nil
}

type episodeCapable struct {
	chainTestProvider
}

func (e episodeCapable) GetEpisodeDetails(_ context.Context, _ string, _, _ int) (*domainmetadata.MetadataResult, error) {
	return nil, nil
}

type seasonCapable struct {
	chainTestProvider
}

func (s seasonCapable) GetSeasonDetails(_ context.Context, _ string, _ int) (*domainmetadata.MetadataResult, error) {
	return nil, nil
}

func TestProviderChain_NilPrimary(t *testing.T) {
	c := NewProviderChain(nil)
	if c.Primary() != nil {
		t.Fatal("expected nil primary for empty chain")
	}
	if c.Supplementary() != nil {
		t.Fatal("expected nil supplementary")
	}
	if _, ok := c.TranslationProvider(); ok {
		t.Fatal("empty chain must not report a translation provider")
	}
}

func TestProviderChain_PrimaryAndSupplementary(t *testing.T) {
	primary := chainTestProvider{name: "tmdb"}
	s1 := chainTestProvider{name: "omdb"}
	s2 := chainTestProvider{name: "tvmaze"}
	c := NewProviderChain(primary, s1, s2)

	if c.Primary().Name() != "tmdb" {
		t.Fatalf("primary = %q", c.Primary().Name())
	}
	supp := c.Supplementary()
	if len(supp) != 2 || supp[0].Name() != "omdb" || supp[1].Name() != "tvmaze" {
		t.Fatalf("supplementary = %v", supp)
	}
	if len(c.All()) != 3 {
		t.Fatalf("all len = %d, want 3", len(c.All()))
	}
}

func TestProviderChain_FirstNonNil_WalksUntilMatch(t *testing.T) {
	calls := []string{}
	c := NewProviderChain(
		chainTestProvider{name: "a"},
		chainTestProvider{name: "b"},
		chainTestProvider{name: "c"},
	)
	hit := "b"
	match := &domainmetadata.MetadataResult{Title: "match"}
	got := c.FirstNonNil(func(p domainmetadata.MetadataProvider) *domainmetadata.MetadataResult {
		calls = append(calls, p.Name())
		if p.Name() == hit {
			return match
		}
		return nil
	})
	if got != match {
		t.Fatal("expected match result")
	}
	if len(calls) != 2 || calls[0] != "a" || calls[1] != "b" {
		t.Fatalf("calls = %v - chain must short-circuit after first match", calls)
	}
}

func TestProviderChain_CapabilityFilters(t *testing.T) {
	primary := translationCapable{chainTestProvider{name: "tmdb"}}
	s1 := episodeCapable{chainTestProvider{name: "ep"}}
	s2 := seasonCapable{chainTestProvider{name: "sea"}}
	s3 := chainTestProvider{name: "plain"}

	c := NewProviderChain(primary, s1, s2, s3)

	if _, ok := c.TranslationProvider(); !ok {
		t.Fatal("expected translation provider from primary")
	}
	if len(c.EpisodeCapable()) != 1 {
		t.Fatalf("episode-capable = %d, want 1", len(c.EpisodeCapable()))
	}
	if len(c.SeasonCapable()) != 1 {
		t.Fatalf("season-capable = %d, want 1", len(c.SeasonCapable()))
	}
}

func TestProviderChain_SkipsNilSupplementaries(t *testing.T) {
	c := NewProviderChain(chainTestProvider{name: "tmdb"}, nil, chainTestProvider{name: "omdb"})
	if len(c.All()) != 2 {
		t.Fatalf("all = %d, want 2 (nil skipped)", len(c.All()))
	}
}
