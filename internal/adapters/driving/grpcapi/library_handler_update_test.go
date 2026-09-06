package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newUpdateLibHandler() (*libraryHandler, *libfakes.FakeLibraryCommandRepo, *libfakes.FakeLibraryQueryRepo) {
	libCmd := libfakes.NewFakeLibraryCommandRepo()
	libQuery := libfakes.NewFakeLibraryQueryRepo()
	mediaCmd := libfakes.NewFakeMediaItemCommandRepo()
	mediaQuery := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	searchIdx := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()

	cmdSvc := applibrary.NewCommandService(
		libCmd, libQuery, mediaCmd, streamRepo,
		personRepo, mediaPersonRepo, scanner, prober, searchIdx, bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(),
		"/tmp/test-trickplay", sharedfakes.NewFakeServerConfigRepository(),
	)
	querySvc := applibrary.NewQueryService(libQuery, mediaQuery, streamRepo, searchIdx)

	return &libraryHandler{cmd: cmdSvc, query: querySvc}, libCmd, libQuery
}

// UpdateLibrary: empty id → InvalidArgument.
func TestGrpcLibraryUpdate_EmptyID(t *testing.T) {
	t.Parallel()
	h, _, _ := newUpdateLibHandler()
	_, err := h.UpdateLibrary(context.Background(), &eveyv1.UpdateLibraryRequest{})
	if err == nil {
		t.Fatal("want error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("code: want InvalidArgument, got %v", st.Code())
	}
}

// UpdateLibrary: library not found → NotFound.
func TestGrpcLibraryUpdate_NotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := newUpdateLibHandler()
	_, err := h.UpdateLibrary(context.Background(), &eveyv1.UpdateLibraryRequest{Id: "missing"})
	if err == nil {
		t.Fatal("want error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("code: want NotFound, got %v", st.Code())
	}
}

// UpdateLibrary: invalid path (not under /media) → InvalidArgument.
func TestGrpcLibraryUpdate_InvalidPath(t *testing.T) {
	t.Parallel()
	h, _, libQuery := newUpdateLibHandler()
	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Old", Type: "movie", Paths: []string{"/media/old"}})

	_, err := h.UpdateLibrary(context.Background(), &eveyv1.UpdateLibraryRequest{
		Id:    "lib-1",
		Paths: []string{"/etc/passwd"},
	})
	if err == nil {
		t.Fatal("want error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("code: want InvalidArgument, got %v", st.Code())
	}
}

// UpdateLibrary: happy path updates name + paths.
func TestGrpcLibraryUpdate_Success(t *testing.T) {
	t.Parallel()
	h, libCmd, libQuery := newUpdateLibHandler()
	// Type set: a stored library always has one (CreateLibrary validates), and
	// UpdateLibrary now re-checks the same invariant.
	lib := &library.Library{ID: "lib-2", Name: "Old", Type: "movie", Paths: []string{"/media/old"}}
	libQuery.Seed(lib)
	libCmd.Seed(lib)

	newName := "Renamed"
	resp, err := h.UpdateLibrary(context.Background(), &eveyv1.UpdateLibraryRequest{
		Id:    "lib-2",
		Name:  &newName,
		Paths: []string{"/media/new"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Name != "Renamed" {
		t.Fatalf("name: want Renamed, got %q", resp.Name)
	}
	if len(resp.Paths) != 1 || resp.Paths[0] != "/media/new" {
		t.Fatalf("paths: %v", resp.Paths)
	}
}
