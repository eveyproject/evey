package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeReindexer records the libraryID it was asked to reindex and signals it on
// a channel so the test can synchronise on the handler's background goroutine
// (no sleep).
type fakeReindexer struct {
	called chan string
	n      int
	err    error
}

func (f *fakeReindexer) ReindexSearch(_ context.Context, libraryID string, _ func(done, total int)) (int, error) {
	f.called <- libraryID
	return f.n, f.err
}

func TestSearchReindexHandler_202AndTriggersAsyncWithLibraryID(t *testing.T) {
	f := &fakeReindexer{called: make(chan string, 1), n: 42}
	h := searchReindexHandler(f, context.Background())

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/api/v1/admin/search/reindex?library_id=lib-42", nil))

	// Returns 202 immediately - the sweep can touch thousands of items.
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Contains(t, rec.Body.String(), "search reindex started")

	// The background goroutine ran ReindexSearch with the query's library_id.
	select {
	case got := <-f.called:
		assert.Equal(t, "lib-42", got)
	case <-time.After(2 * time.Second):
		t.Fatal("ReindexSearch was not invoked asynchronously")
	}
}

func TestSearchReindexHandler_NoLibraryID_FullReindex(t *testing.T) {
	f := &fakeReindexer{called: make(chan string, 1)}
	h := searchReindexHandler(f, context.Background())

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/api/v1/admin/search/reindex", nil))

	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case got := <-f.called:
		assert.Empty(t, got, "absent library_id → full reindex (empty id)")
	case <-time.After(2 * time.Second):
		t.Fatal("ReindexSearch was not invoked")
	}
}
