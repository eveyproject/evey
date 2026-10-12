package metadata

import (
	"context"
	"errors"
	"log/slog"
	"time"

	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

// RetryProvider wraps a MetadataProvider and retries Search / GetByProviderID
// on transient failures with exponential backoff.
//
// Scope: only the base MetadataProvider surface gets retried. Capability
// calls (translations, season/episode details) are reached via Unwrap
// and bypass retry - acceptable because they're one-shot per item and
// the scan hot path is Search/GetByProviderID.
type RetryProvider struct {
	inner       domainmetadata.MetadataProvider
	maxAttempts int
	baseDelay   time.Duration
}

// NewRetryProvider wraps inner. maxAttempts includes the initial call;
// a value of 1 disables retry. baseDelay is doubled each attempt.
func NewRetryProvider(inner domainmetadata.MetadataProvider, maxAttempts int, baseDelay time.Duration) *RetryProvider {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return &RetryProvider{inner: inner, maxAttempts: maxAttempts, baseDelay: baseDelay}
}

func (r *RetryProvider) Name() string                            { return r.inner.Name() }
func (r *RetryProvider) Unwrap() domainmetadata.MetadataProvider { return r.inner }

func (r *RetryProvider) Search(ctx context.Context, query string, year *int, mediaType string) ([]domainmetadata.MetadataResult, error) {
	var (
		last []domainmetadata.MetadataResult
		err  error
	)
	for attempt := 0; attempt < r.maxAttempts; attempt++ {
		last, err = r.inner.Search(ctx, query, year, mediaType)
		if err == nil {
			return last, nil
		}
		if !isRetryable(ctx, err) || attempt == r.maxAttempts-1 {
			return last, err
		}
		if waitErr := sleepBackoff(ctx, r.baseDelay, attempt); waitErr != nil {
			return nil, waitErr
		}
		slog.DebugContext(ctx, "retry provider search",
			slog.String("provider", r.inner.Name()),
			slog.String("query", query),
			slog.Int("attempt", attempt+1),
			slog.Any("err", err))
	}
	return last, err
}

func (r *RetryProvider) GetByProviderID(ctx context.Context, providerID, mediaType string) (*domainmetadata.MetadataResult, error) {
	var (
		last *domainmetadata.MetadataResult
		err  error
	)
	for attempt := 0; attempt < r.maxAttempts; attempt++ {
		last, err = r.inner.GetByProviderID(ctx, providerID, mediaType)
		if err == nil {
			return last, nil
		}
		if !isRetryable(ctx, err) || attempt == r.maxAttempts-1 {
			return last, err
		}
		if waitErr := sleepBackoff(ctx, r.baseDelay, attempt); waitErr != nil {
			return nil, waitErr
		}
		slog.DebugContext(ctx, "retry provider get-by-id",
			slog.String("provider", r.inner.Name()),
			slog.String("provider_id", providerID),
			slog.Int("attempt", attempt+1),
			slog.Any("err", err))
	}
	return last, err
}

// isRetryable treats context cancellation/deadline as permanent. Other
// errors are retried because providers surface empty results (not
// errors) for legitimate "not found" cases - only plumbing failures
// (network, 5xx) bubble up as errors here.
func isRetryable(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	return true
}

func sleepBackoff(ctx context.Context, base time.Duration, attempt int) error {
	delay := base << attempt
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}
