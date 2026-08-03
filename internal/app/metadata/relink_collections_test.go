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
func (f *recordingLinker) ListMoviesWithoutCollection(_ context.Context, limit int) ([]domainlib.UnlinkedMovie, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit > 0 && len(f.unlinked) > limit {
		return f.unlinked[:limit], nil
	}
	return f.unlinked, nil
}

// The repair that answers the field complaint: the sequels ARE on the server,
// and their saga lists one title, because membership is only written while a
// title is being enriched.
func TestRelinkCollections_LinksMoviesWhoseSagaIsKnown(t *testing.T) {
	linker := &recordingLinker{
		unlinked: []domainlib.UnlinkedMovie{
			{MediaItemID: "mi-reloaded", ProviderID: "604"},
			{MediaItemID: "mi-revolutions", ProviderID: "605"},
		},
		col: &domainlib.Collection{ID: "col-matrix", Name: "Matrix - Saga"},
	}
	provider := &fakeProvider{detailResult: &domain.MetadataResult{
		Collection: &domain.CollectionRef{TmdbID: 2344, Name: "Matrix Collection"},
	}}
	e, _, _, _, _, _ := newTestEnricher(provider, nil, linker)

	rep, err := e.RelinkCollections(context.Background(), 100)
	if err != nil {
		t.Fatalf("RelinkCollections: %v", err)
	}
	if rep.Scanned != 2 || rep.Linked != 2 {
		t.Errorf("report = %+v, want scanned 2 / linked 2", rep)
	}
	want := []string{"col-matrix|mi-reloaded", "col-matrix|mi-revolutions"}
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

func TestRelinkCollections_ListFailureIsReturned(t *testing.T) {
	linker := &recordingLinker{listErr: errors.New("db down")}
	e, _, _, _, _, _ := newTestEnricher(&fakeProvider{}, nil, linker)
	if _, err := e.RelinkCollections(context.Background(), 100); err == nil {
		t.Fatal("expected the list failure to surface")
	}
}
