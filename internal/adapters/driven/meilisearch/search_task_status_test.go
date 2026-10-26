package meilisearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// fakeMeili answers like a Meilisearch server: every write is accepted as an
// enqueued task, and the task FAILS when its batch holds a document whose id
// starts with "bad" - the engine rejects the whole batch, not just that doc.
type fakeMeili struct {
	mu      sync.Mutex
	tasks   map[int64]bool // uid -> failed
	indexed map[string]bool
	next    int64
}

func newFakeMeili(t *testing.T) (*fakeMeili, *httptest.Server) {
	f := &fakeMeili{tasks: map[int64]bool{}, indexed: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeMeili) enqueue(w http.ResponseWriter, failed bool) {
	f.mu.Lock()
	f.next++
	uid := f.next
	f.tasks[uid] = failed
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = fmt.Fprintf(w, `{"taskUid":%d,"indexUid":"media_items","status":"enqueued","type":"documentAdditionOrUpdate"}`, uid)
}

func (f *fakeMeili) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/indexes/media_items":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"uid":"media_items","primaryKey":"id"}`))
	case r.URL.Path == "/indexes/media_items/settings":
		f.enqueue(w, false)
	case r.Method == http.MethodPost && r.URL.Path == "/indexes/media_items/documents":
		var docs []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&docs)
		failed := false
		for _, d := range docs {
			if strings.HasPrefix(fmt.Sprint(d["id"]), "bad") {
				failed = true
			}
		}
		if !failed {
			f.mu.Lock()
			for _, d := range docs {
				f.indexed[fmt.Sprint(d["id"])] = true
			}
			f.mu.Unlock()
		}
		f.enqueue(w, failed)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/tasks/"):
		var uid int64
		_, _ = fmt.Sscan(strings.TrimPrefix(r.URL.Path, "/tasks/"), &uid)
		f.mu.Lock()
		failed := f.tasks[uid]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if failed {
			_, _ = fmt.Fprintf(w, `{"uid":%d,"indexUid":"media_items","status":"failed","type":"documentAdditionOrUpdate","error":{"message":"invalid document","code":"invalid_document_fields","type":"invalid_request","link":""}}`, uid)
			return
		}
		_, _ = fmt.Fprintf(w, `{"uid":%d,"indexUid":"media_items","status":"succeeded","type":"documentAdditionOrUpdate"}`, uid)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeMeili) indexedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for id := range f.indexed {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func items(ids ...string) []*library.MediaItem {
	out := make([]*library.MediaItem, len(ids))
	for i, id := range ids {
		out[i] = &library.MediaItem{ID: shared.MediaItemID(id), Title: "Title " + id, Type: library.MovieItem}
	}
	return out
}

func TestIndexMediaItemsBatch_FailedTaskIsReportedAndBisected(t *testing.T) {
	f, srv := newFakeMeili(t)
	a, err := NewSearchAdapter(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}

	err = a.IndexMediaItemsBatch(context.Background(), items("a", "b", "bad1", "c", "d"))
	var rejected interface{ RejectedIDs() []string }
	if !errors.As(err, &rejected) {
		t.Fatalf("a batch whose Meilisearch task failed must return a rejection, got: %v", err)
	}
	if got := rejected.RejectedIDs(); len(got) != 1 || got[0] != "bad1" {
		t.Fatalf("rejected ids = %v, want [bad1]", got)
	}
	if got := strings.Join(f.indexedIDs(), ","); got != "a,b,c,d" {
		t.Fatalf("indexed = %s, want the accepted documents a,b,c,d", got)
	}
}

func TestIndexMediaItem_FailedTaskIsAnError(t *testing.T) {
	_, srv := newFakeMeili(t)
	a, err := NewSearchAdapter(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.IndexMediaItem(context.Background(), items("bad2")[0]); err == nil {
		t.Fatal("a document whose Meilisearch task failed must not pass as indexed")
	}
	if err := a.IndexMediaItem(context.Background(), items("ok")[0]); err != nil {
		t.Fatalf("succeeded task: %v", err)
	}
}

func TestRemoveLibrary_DeletesByLibraryFilter(t *testing.T) {
	var gotPath, gotFilter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/indexes/media_items":
			_, _ = w.Write([]byte(`{"uid":"media_items","primaryKey":"id"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/indexes/media_items/documents/delete":
			var body struct {
				Filter string `json:"filter"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotPath, gotFilter = r.URL.Path, body.Filter
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid":7,"indexUid":"media_items","status":"enqueued"}`))
		case strings.HasPrefix(r.URL.Path, "/tasks/"):
			_, _ = w.Write([]byte(`{"uid":7,"status":"succeeded"}`))
		default:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"taskUid":1,"status":"enqueued"}`))
		}
	}))
	t.Cleanup(srv.Close)
	a, err := NewSearchAdapter(srv.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveLibrary(context.Background(), "lib-1'x"); err != nil {
		t.Fatal(err)
	}
	if gotPath == "" || gotFilter != `library_id = 'lib-1\'x'` {
		t.Fatalf("delete by filter = %q %q, want the escaped library_id filter", gotPath, gotFilter)
	}
}
