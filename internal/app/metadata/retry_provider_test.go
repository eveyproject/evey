package metadata

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

type decoratorFakeProvider struct {
	name         string
	searchCalls  int32
	getByIDCalls int32
	searchErrs   []error
	getByIDErrs  []error
	searchResult []domainmetadata.MetadataResult
	idResult     *domainmetadata.MetadataResult
}

func (f *decoratorFakeProvider) Name() string { return f.name }

func (f *decoratorFakeProvider) Search(_ context.Context, _ string, _ *int, _ string) ([]domainmetadata.MetadataResult, error) {
	n := atomic.AddInt32(&f.searchCalls, 1)
	if int(n)-1 < len(f.searchErrs) {
		if err := f.searchErrs[n-1]; err != nil {
			return nil, err
		}
	}
	return f.searchResult, nil
}

func (f *decoratorFakeProvider) GetByProviderID(_ context.Context, _ string, _ string) (*domainmetadata.MetadataResult, error) {
	n := atomic.AddInt32(&f.getByIDCalls, 1)
	if int(n)-1 < len(f.getByIDErrs) {
		if err := f.getByIDErrs[n-1]; err != nil {
			return nil, err
		}
	}
	return f.idResult, nil
}

func TestRetryProvider_SucceedsAfterTransientFailures(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:         "tmdb",
		searchErrs:   []error{errors.New("5xx"), errors.New("timeout"), nil},
		searchResult: []domainmetadata.MetadataResult{{Title: "ok"}},
	}
	r := NewRetryProvider(inner, 3, time.Microsecond)
	got, err := r.Search(context.Background(), "q", nil, "movie")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].Title != "ok" {
		t.Fatalf("got = %v", got)
	}
	if inner.searchCalls != 3 {
		t.Fatalf("search calls = %d, want 3", inner.searchCalls)
	}
}

func TestRetryProvider_GivesUpAfterMaxAttempts(t *testing.T) {
	boom := errors.New("boom")
	inner := &decoratorFakeProvider{
		name:       "omdb",
		searchErrs: []error{boom, boom, boom},
	}
	r := NewRetryProvider(inner, 3, time.Microsecond)
	_, err := r.Search(context.Background(), "q", nil, "movie")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if inner.searchCalls != 3 {
		t.Fatalf("search calls = %d, want 3", inner.searchCalls)
	}
}

func TestRetryProvider_ContextCanceledDoesNotRetry(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:       "tvmaze",
		searchErrs: []error{context.Canceled},
	}
	r := NewRetryProvider(inner, 5, time.Microsecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Search(ctx, "q", nil, "movie")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if inner.searchCalls != 1 {
		t.Fatalf("search calls = %d, want 1 (no retry on cancel)", inner.searchCalls)
	}
}

func TestRetryProvider_GetByProviderID_RetriesAndSucceeds(t *testing.T) {
	result := &domainmetadata.MetadataResult{Title: "found"}
	inner := &decoratorFakeProvider{
		name:        "imdb",
		getByIDErrs: []error{errors.New("net"), nil},
		idResult:    result,
	}
	r := NewRetryProvider(inner, 3, time.Microsecond)
	got, err := r.GetByProviderID(context.Background(), "tt1", "movie")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != result {
		t.Fatal("expected result pointer")
	}
	if inner.getByIDCalls != 2 {
		t.Fatalf("calls = %d, want 2", inner.getByIDCalls)
	}
}

// GetByProviderID gives up after maxAttempts like Search does: three calls, the
// last error returned. Only its success path was tested (mutation 2026-09-28).
func TestRetryProvider_GetByProviderID_GivesUpAfterMaxAttempts(t *testing.T) {
	boom := errors.New("boom")
	inner := &decoratorFakeProvider{
		name:        "imdb",
		getByIDErrs: []error{boom, boom, boom, nil},
		idResult:    &domainmetadata.MetadataResult{Title: "too late"},
	}
	r := NewRetryProvider(inner, 3, time.Microsecond)
	if _, err := r.GetByProviderID(context.Background(), "tt1", "movie"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if inner.getByIDCalls != 3 {
		t.Fatalf("calls = %d, want 3", inner.getByIDCalls)
	}
}

func TestRetryProvider_Unwrap(t *testing.T) {
	inner := &decoratorFakeProvider{name: "tmdb"}
	r := NewRetryProvider(inner, 3, time.Microsecond)
	if r.Unwrap() != inner {
		t.Fatal("Unwrap must return inner")
	}
	if r.Name() != "tmdb" {
		t.Fatalf("name = %q", r.Name())
	}
}

func TestRetryProvider_MaxAttemptsFloor(t *testing.T) {
	inner := &decoratorFakeProvider{
		name:       "x",
		searchErrs: []error{errors.New("once")},
	}
	r := NewRetryProvider(inner, 0, time.Microsecond)
	_, err := r.Search(context.Background(), "q", nil, "movie")
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.searchCalls != 1 {
		t.Fatalf("calls = %d, want 1", inner.searchCalls)
	}
}
