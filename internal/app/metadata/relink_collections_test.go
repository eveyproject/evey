package metadata

import (
	"context"
	"errors"
	"testing"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	domain "github.com/evey-project/evey/internal/domain/metadata"
)

// recordingLinker records what the repair actually linked, and serves the
// work list the postgres query would return.
type recordingLinker struct {
	unlinked    []domainlib.UnlinkedMovie
	listErr     error
	col         *domainlib.Collection
	upsertErr   error
	linkFailFor string
	links       []string // "<collectionID>|<mediaItemID>"
	pageSize    int      // rows served per call, 0 = every remaining row
	pages       []string // the cursor each call was made with
}

func (f *recordingLinker) Upsert(_ context.Context, tmdbID int, name, _, _, _ string) (*domainlib.Collection, error) {
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	return f.col, nil
}
func (f *recordingLinker) LinkMediaItem(_ context.Context, collectionID, mediaItemID string, _ int) error {
	if mediaItemID == f.linkFailFor {
		return errors.New("link failed")
	}
	f.links = append(f.links, collectionID+"|"+mediaItemID)
	return nil
}

// Mirrors the postgres query: rows strictly after the cursor, in id order,
// capped by limit. Serving pages is the whole point - a standalone film is
// never removed from the work list, so a sweep that cannot advance past its
// first page repairs nothing beyond it, however many times it is re-run.
func (f *recordingLinker) ListMoviesWithoutCollection(_ context.Context, afterID string, limit int) ([]domainlib.UnlinkedMovie, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.pages = append(f.pages, afterID)
	var out []domainlib.UnlinkedMovie
	for _, m := range f.unlinked {
		if string(m.MediaItemID) <= afterID {
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
		if f.pageSize > 0 && len(out) >= f.pageSize {
			break
		}
	}
	return out, nil
}

// The repair that answers the field complaint: the sequels ARE on the server,
// and their saga lists one title, because membership is only written while a
// title is being enriched.
func TestRelinkCollections_LinksMoviesWhoseSagaIsKnown(t *testing.T) {
	linker := &recordingLinker{
		unlinked: []domainlib.UnlinkedMovie{
			{MediaItemID: "mi-sequel-a", ProviderID: "604"},
			{MediaItemID: "mi-sequel-b", ProviderID: "605"},
		},
		col: &domainlib.Collection{ID: "col-saga", Name: "Some Saga"},
	}
	provider := &fakeProvider{detailResult: &domain.MetadataResult{
		Collection: &domain.CollectionRef{TmdbID: 2344, Name: "Some Saga Collection"},
	}}
	e, _, _, _, _, _ := newTestEnricher(provider, nil, linker)

	rep, err := e.RelinkCollections(context.Background(), 100)
	if err != nil {
		t.Fatalf("RelinkCollections: %v", err)
	}
	if rep.Scanned != 2 || rep.Linked != 2 {
		t.Errorf("report = %+v, want scanned 2 / linked 2", rep)
	}
	want := []string{"col-saga|mi-sequel-a", "col-saga|mi-sequel-b"}
	if len(linker.links) != 2 || linker.links[0] != want[0] || linker.links[1] != want[1] {
		t.Errorf("links = %v, want %v", linker.links, want)
	}
}

// A movie that belongs to no saga is scanned and left alone - it must not be
// reported as linked, and must not error the run.
func TestRelinkCollections_SkipsMoviesWithoutASaga(t *testing.T) {
	linker := &recordingLinker{
		unlinked: []domainlib.UnlinkedMovie{{MediaItemID: "mi-standalone", ProviderID: "42"}},
		col:      &domainlib.Collection{ID: "col-x"},
	}
	provider := &fakeProvider{detailResult: &domain.MetadataResult{}} // no Collection ref
	e, _, _, _, _, _ := newTestEnricher(provider, nil, linker)

	rep, err := e.RelinkCollections(context.Background(), 100)
	if err != nil {
		t.Fatalf("RelinkCollections: %v", err)
	}
	if rep.Scanned != 1 || rep.Linked != 0 {
		t.Errorf("report = %+v, want scanned 1 / linked 0", rep)
	}
	if len(linker.links) != 0 {
		t.Errorf("nothing must be linked, got %v", linker.links)
	}
}

// One failing title must not abort the sweep: the next ones still get
// repaired, and the failure is counted.
func TestRelinkCollections_OneFailureDoesNotAbortTheSweep(t *testing.T) {
	linker := &recordingLinker{
		unlinked: []domainlib.UnlinkedMovie{
			{MediaItemID: "mi-a", ProviderID: "1"},
			{MediaItemID: "mi-b", ProviderID: "2"},
		},
		col:         &domainlib.Collection{ID: "col-y"},
		linkFailFor: "mi-a",
	}
	provider := &fakeProvider{detailResult: &domain.MetadataResult{
		Collection: &domain.CollectionRef{TmdbID: 7, Name: "S"},
	}}
	e, _, _, _, _, _ := newTestEnricher(provider, nil, linker)

	rep, err := e.RelinkCollections(context.Background(), 100)
	if err != nil {
		t.Fatalf("RelinkCollections must not fail the run: %v", err)
	}
	if rep.Scanned != 2 || rep.Linked != 1 || rep.Failed != 1 {
		t.Errorf("report = %+v, want scanned 2 / linked 1 / failed 1", rep)
	}
}

// An empty work list is the steady state once the repair has run: no calls,
// no error.
func TestRelinkCollections_NothingToDo(t *testing.T) {
	linker := &recordingLinker{}
	e, _, _, _, _, _ := newTestEnricher(&fakeProvider{}, nil, linker)
	rep, err := e.RelinkCollections(context.Background(), 100)
	if err != nil {
		t.Fatalf("RelinkCollections: %v", err)
	}
	if rep.Scanned != 0 || rep.Linked != 0 {
		t.Errorf("report = %+v, want an all-zero report", rep)
	}
}

// The sweep must walk the whole work list, not just its first page. A film
// that belongs to no saga stays in the list for ever, so a page-sized sweep
// keeps re-scanning the same standalone titles and never reaches the ones
// behind them - re-running it does not help, which is what the "re-run to
// continue" story got wrong.
func TestRelinkCollections_WalksPastTheFirstPage(t *testing.T) {
	linker := &recordingLinker{
		pageSize: 2,
		unlinked: []domainlib.UnlinkedMovie{
			{MediaItemID: "mi-1", ProviderID: "1"},
			{MediaItemID: "mi-2", ProviderID: "2"},
			{MediaItemID: "mi-3", ProviderID: "3"},
			{MediaItemID: "mi-4", ProviderID: "4"},
			{MediaItemID: "mi-5", ProviderID: "5"},
		},
		col: &domainlib.Collection{ID: "col-z"},
	}
	provider := &fakeProvider{detailResult: &domain.MetadataResult{
		Collection: &domain.CollectionRef{TmdbID: 9, Name: "Z"},
	}}
	e, _, _, _, _, _ := newTestEnricher(provider, nil, linker)

	rep, err := e.RelinkCollections(context.Background(), 0)
	if err != nil {
		t.Fatalf("RelinkCollections: %v", err)
	}
	if rep.Scanned != 5 || rep.Linked != 5 {
		t.Errorf("report = %+v, want scanned 5 / linked 5", rep)
	}
	// Each call must start after the last title of the previous page, and the
	// last one must come back empty for the sweep to know it is done.
	want := []string{"", "mi-2", "mi-4", "mi-5"}
	if len(linker.pages) != len(want) {
		t.Fatalf("cursors = %v, want %v", linker.pages, want)
	}
	for i, c := range want {
		if linker.pages[i] != c {
			t.Errorf("cursor %d = %q, want %q", i, linker.pages[i], c)
		}
	}
}

// A caller-set limit still caps the sweep, and must not be exceeded by a
// page that would overshoot it.
func TestRelinkCollections_LimitCapsTheSweep(t *testing.T) {
	linker := &recordingLinker{
		pageSize: 2,
		unlinked: []domainlib.UnlinkedMovie{
			{MediaItemID: "mi-1", ProviderID: "1"},
			{MediaItemID: "mi-2", ProviderID: "2"},
			{MediaItemID: "mi-3", ProviderID: "3"},
			{MediaItemID: "mi-4", ProviderID: "4"},
		},
		col: &domainlib.Collection{ID: "col-z"},
	}
	provider := &fakeProvider{detailResult: &domain.MetadataResult{
		Collection: &domain.CollectionRef{TmdbID: 9, Name: "Z"},
	}}
	e, _, _, _, _, _ := newTestEnricher(provider, nil, linker)

	rep, err := e.RelinkCollections(context.Background(), 3)
	if err != nil {
		t.Fatalf("RelinkCollections: %v", err)
	}
	if rep.Scanned != 3 {
		t.Errorf("scanned = %d, want 3", rep.Scanned)
	}
}

func TestRelinkCollections_ListFailureIsReturned(t *testing.T) {
	linker := &recordingLinker{listErr: errors.New("db down")}
	e, _, _, _, _, _ := newTestEnricher(&fakeProvider{}, nil, linker)
	if _, err := e.RelinkCollections(context.Background(), 100); err == nil {
		t.Fatal("expected the list failure to surface")
	}
}
