package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"google.golang.org/grpc"
)

type scanLibStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*eveyv1.ScanProgress
}

func (s *scanLibStream) Send(p *eveyv1.ScanProgress) error {
	s.sent = append(s.sent, p)
	return nil
}
func (s *scanLibStream) Context() context.Context { return s.ctx }

func newScanLibHandler() (*libraryHandler, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeLibraryCommandRepo) {
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

	return &libraryHandler{
		cmd: cmdSvc, query: querySvc,
		translator: eveyi18n.NewEmpty(),
	}, libQuery, libCmd
}

// ScanLibrary: success → one progress event emitted with COMPLETE phase.
func TestGrpcScanLibrary_Success(t *testing.T) {
	t.Parallel()
	h, libQuery, libCmd := newScanLibHandler()
	lib := &library.Library{ID: "lib-1", Name: "L", Type: "movie", Paths: []string{"/media/x"}}
	libQuery.Seed(lib)
	libCmd.Seed(lib)

	ss := &scanLibStream{ctx: context.Background()}
	err := h.ScanLibrary(&eveyv1.ScanLibraryRequest{Id: "lib-1"}, ss)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ss.sent) != 1 {
		t.Fatalf("want 1 progress msg, got %d", len(ss.sent))
	}
	if ss.sent[0].Phase != eveyv1.ScanPhase_SCAN_PHASE_COMPLETE {
		t.Fatalf("phase: want COMPLETE, got %v", ss.sent[0].Phase)
	}
}

// ScanLibrary: non-existent library → error.
func TestGrpcScanLibrary_NotFound(t *testing.T) {
	t.Parallel()
	h, _, _ := newScanLibHandler()
	ss := &scanLibStream{ctx: context.Background()}
	err := h.ScanLibrary(&eveyv1.ScanLibraryRequest{Id: "missing"}, ss)
	if err == nil {
		t.Fatal("want error")
	}
}

type failingRootScanner struct{ failing string }

func (f failingRootScanner) ScanDirectory(_ context.Context, _, dir string) (int, error) {
	if dir == f.failing {
		return 0, errors.New("share unmounted")
	}
	return 3, nil
}

// A music root the scan could not read reaches the client: the COMPLETE
// message names it, so the admin sees a partial scan, not a clean 100 %.
func TestGrpcScanLibrary_ReportsTheRootsItCouldNotRead(t *testing.T) {
	t.Parallel()
	h, libQuery, libCmd := newScanLibHandler()
	lib := &library.Library{ID: "lib-m", Name: "M", Type: "music", Paths: []string{"/music/a", "/music/b"}}
	libQuery.Seed(lib)
	libCmd.Seed(lib)
	h.cmd.WithMusicScanner(failingRootScanner{failing: "/music/a"})

	ss := &scanLibStream{ctx: context.Background()}
	if err := h.ScanLibrary(&eveyv1.ScanLibraryRequest{Id: "lib-m"}, ss); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ss.sent) != 1 {
		t.Fatalf("want 1 progress msg, got %d", len(ss.sent))
	}
	if got := ss.sent[0].FailedPaths; len(got) != 1 || got[0] != "/music/a" {
		t.Fatalf("failed paths = %v, want [/music/a]", got)
	}
}
