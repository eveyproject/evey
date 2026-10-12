package introproviders

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// ChainedProvider implements domainlibrary.IntroMetadataProvider by
// trying each Client in order until one returns non-empty data. Per-
// source rate-limited errors are sticky for the configured cooldown so
// repeated calls don't burn through the daily quota.
type ChainedProvider struct {
	clients  []Client
	resolver *AncestorResolver
	cooldown time.Duration

	mu          sync.Mutex
	rateLimited map[string]time.Time // source → cooldown-until timestamp
}

// NewChainedProvider returns a provider that fans out to clients in
// declaration order. Empty resp from one client falls through to the
// next; first non-empty wins.
func NewChainedProvider(resolver *AncestorResolver, clients ...Client) *ChainedProvider {
	return &ChainedProvider{
		clients:     clients,
		resolver:    resolver,
		cooldown:    10 * time.Minute,
		rateLimited: make(map[string]time.Time),
	}
}

// WithCooldown overrides the post-429 sticky cooldown. Default 10m: long
// enough to clear the burst window (10s) and to mute the daily quota
// failure mode without freezing the chain for the whole day. Tests
// pass a tiny duration to exercise the path.
func (p *ChainedProvider) WithCooldown(d time.Duration) *ChainedProvider {
	p.cooldown = d
	return p
}

// GetSegments resolves the lookup, then tries each client in order.
// Errors from one client are logged + suppressed so a flaky upstream
// doesn't poison the chain. Rate-limited sources are skipped until
// their cooldown expires.
func (p *ChainedProvider) GetSegments(ctx context.Context, item *domainlibrary.MediaItem) ([]domainlibrary.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "introproviders.chain.GetSegments")
	defer span.End()

	lookup, err := p.resolver.Resolve(ctx, item)
	if err != nil {
		if errors.Is(err, ErrLookupUnresolvable) {
			return nil, nil // no IDs = silent skip, fall through to chapter pass
		}
		slog.Warn("introproviders.chain: ancestor resolution failed",
			slog.String("media_id", string(item.ID)), slog.Any("err", err))
		return nil, nil
	}

	for _, client := range p.clients {
		if p.isCoolingDown(client.Source()) {
			continue
		}
		segs, fetchErr := client.Fetch(ctx, lookup)
		if fetchErr != nil {
			if rl := (ErrRateLimited{}); errors.As(fetchErr, &rl) {
				p.markCooldown(client.Source(), rl.RetryAfter)
				slog.Warn("introproviders.chain: rate-limited",
					slog.String("source", client.Source()),
					slog.Duration("retry_after", rl.RetryAfter))
				continue
			}
			slog.Warn("introproviders.chain: provider error, falling through",
				slog.String("source", client.Source()),
				slog.Any("err", fetchErr))
			continue
		}
		if len(segs) > 0 {
			return segs, nil
		}
	}
	return nil, nil
}

func (p *ChainedProvider) isCoolingDown(source string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.rateLimited[source]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(p.rateLimited, source)
		return false
	}
	return true
}

func (p *ChainedProvider) markCooldown(source string, retryAfter time.Duration) {
	d := p.cooldown
	if retryAfter > 0 && retryAfter > d {
		d = retryAfter
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rateLimited[source] = time.Now().Add(d)
}
