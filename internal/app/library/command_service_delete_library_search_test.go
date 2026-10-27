package library_test

import (
	"context"
	"testing"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

type libraryPurgingIndex struct {
	*libfakes.FakeSearchIndex
	removedLibraries []string
}

func (s *libraryPurgingIndex) RemoveLibrary(_ context.Context, libraryID string) error {
	s.removedLibraries = append(s.removedLibraries, libraryID)
	return nil
}

// The database cascades a deleted library's media away; their search
// documents must go with them or the titles stay findable.
func TestDeleteLibrary_PurgesTheLibrarySearchDocuments(t *testing.T) {
	libCmd := libfakes.NewFakeLibraryCommandRepo()
	search := &libraryPurgingIndex{FakeSearchIndex: libfakes.NewFakeSearchIndex()}
	svc := applib.NewCommandService(libCmd, libfakes.NewFakeLibraryQueryRepo(), libfakes.NewFakeMediaItemCommandRepo(),
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		libfakes.NewFakeFileScanner(), libfakes.NewFakeStreamProber(), search, sharedfakes.NewFakeEventBus(),
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", nil,
	)
	_ = libCmd.Create(context.Background(), &library.Library{ID: "l-gone"})

	if err := svc.DeleteLibrary(context.Background(), "l-gone"); err != nil {
		t.Fatal(err)
	}
	if len(search.removedLibraries) != 1 || search.removedLibraries[0] != "l-gone" {
		t.Fatalf("search documents purged for %v, want [l-gone]", search.removedLibraries)
	}
}
