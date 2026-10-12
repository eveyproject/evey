package metadata

import (
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

// ProviderChain is an ordered list of metadata providers with capability
// routing. Walk the chain to find the first provider that satisfies a
// query - search fallback, season details, episode details, translations.
//
// Pattern: Chain of Responsibility. Each step is "try this provider; on
// nil/error, fall through to the next." First non-nil wins. Capability
// filters (EpisodeCapable, SeasonCapable, TranslationProvider) drop
// providers that can't answer the specific question so callers don't have
// to type-assert inline.
type ProviderChain struct {
	providers []domainmetadata.MetadataProvider
}

// NewProviderChain builds a chain with the primary provider first,
// followed by the supplementary providers in the order given. A nil
// primary yields an empty chain; nil supplementaries are skipped.
func NewProviderChain(primary domainmetadata.MetadataProvider, supplementary ...domainmetadata.MetadataProvider) *ProviderChain {
	all := make([]domainmetadata.MetadataProvider, 0, 1+len(supplementary))
	if primary != nil {
		all = append(all, primary)
	}
	for _, p := range supplementary {
		if p != nil {
			all = append(all, p)
		}
	}
	return &ProviderChain{providers: all}
}

// Primary returns the first provider, or nil when the chain is empty.
func (c *ProviderChain) Primary() domainmetadata.MetadataProvider {
	if len(c.providers) == 0 {
		return nil
	}
	return c.providers[0]
}

// Supplementary returns every provider after the primary.
func (c *ProviderChain) Supplementary() []domainmetadata.MetadataProvider {
	if len(c.providers) <= 1 {
		return nil
	}
	return c.providers[1:]
}

// All returns every provider in chain order.
func (c *ProviderChain) All() []domainmetadata.MetadataProvider {
	return c.providers
}

// FirstNonNil walks the chain and returns the first non-nil result from
// the selector. This is the core fallback loop extracted from the
// enricher's AutoEnrichItem so callers don't inline `for _, p := range
// supplementary; if result := ...; result != nil { break }`.
func (c *ProviderChain) FirstNonNil(selector func(domainmetadata.MetadataProvider) *domainmetadata.MetadataResult) *domainmetadata.MetadataResult {
	for _, p := range c.providers {
		if result := selector(p); result != nil {
			return result
		}
	}
	return nil
}

// EpisodeCapableProvider pairs a provider with its EpisodeDetailProvider
// view so callers don't re-do the type assertion when iterating.
type EpisodeCapableProvider struct {
	Provider domainmetadata.MetadataProvider
	Detail   domainmetadata.EpisodeDetailProvider
}

// EpisodeCapable returns the subset of providers that implement
// EpisodeDetailProvider, in chain order. Decorated providers are
// peeled via Unwrap so a TMDB wrapped in retry/cache still surfaces
// its episode capability.
func (c *ProviderChain) EpisodeCapable() []EpisodeCapableProvider {
	var out []EpisodeCapableProvider
	for _, p := range c.providers {
		if ep, ok := capabilityFrom[domainmetadata.EpisodeDetailProvider](p); ok {
			out = append(out, EpisodeCapableProvider{Provider: p, Detail: ep})
		}
	}
	return out
}

// SeasonCapableProvider pairs a provider with its SeasonDetailProvider view.
type SeasonCapableProvider struct {
	Provider domainmetadata.MetadataProvider
	Detail   domainmetadata.SeasonDetailProvider
}

// SeasonCapable returns the subset of providers that implement
// SeasonDetailProvider, in chain order. Decorated providers are
// peeled via Unwrap.
func (c *ProviderChain) SeasonCapable() []SeasonCapableProvider {
	var out []SeasonCapableProvider
	for _, p := range c.providers {
		if sp, ok := capabilityFrom[domainmetadata.SeasonDetailProvider](p); ok {
			out = append(out, SeasonCapableProvider{Provider: p, Detail: sp})
		}
	}
	return out
}

// TranslationProvider returns the primary's translation capability, if any.
// Translations only come from the primary because fallback providers
// would carry different canonical titles and confuse the UI. Decorated
// primaries are peeled via Unwrap.
func (c *ProviderChain) TranslationProvider() (domainmetadata.TranslationProvider, bool) {
	p := c.Primary()
	if p == nil {
		return nil, false
	}
	return capabilityFrom[domainmetadata.TranslationProvider](p)
}

// capabilityFrom walks the Unwrap chain to find a provider satisfying T.
// Returns zero value + false when no layer implements T.
func capabilityFrom[T any](p domainmetadata.MetadataProvider) (T, bool) {
	for cur := p; cur != nil; {
		if t, ok := any(cur).(T); ok {
			return t, true
		}
		u, ok := cur.(domainmetadata.ProviderUnwrapper)
		if !ok {
			break
		}
		cur = u.Unwrap()
	}
	var zero T
	return zero, false
}
