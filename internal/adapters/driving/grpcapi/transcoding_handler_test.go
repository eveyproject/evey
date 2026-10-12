package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	tcfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestTranscodingHandler() (*transcodingHandler, *tcfakes.FakeTranscoder, *tcfakes.FakeTranscodeJobRepository, *tcfakes.FakeTranscodeProfileRepository) {
	transcoder := tcfakes.NewFakeTranscoder()
	hwDetector := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	events := sharedfakes.NewFakeEventBus()
	svc := apptranscoding.NewService(transcoder, hwDetector, jobRepo, profileRepo, events, 5)
	return &transcodingHandler{svc: svc, userAccess: newPermissiveTestUserAccess()}, transcoder, jobRepo, profileRepo
}

// newTranscodingHandlerWithAccess wires libQuery + userAccess so the
// defense-in-depth IDOR check (checkJobAccess) actually runs.
func newTranscodingHandlerWithAccess() (
	h *transcodingHandler,
	transcoder *tcfakes.FakeTranscoder,
	jobRepo *tcfakes.FakeTranscodeJobRepository,
	mediaItemRepo *libfakes.FakeMediaItemQueryRepo,
	libAccessRepo *userfakes.FakeLibraryAccessRepository,
) {
	transcoder = tcfakes.NewFakeTranscoder()
	hwDetector := tcfakes.NewFakeHWDetector()
	jobRepo = tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	events := sharedfakes.NewFakeEventBus()
	svc := apptranscoding.NewService(transcoder, hwDetector, jobRepo, profileRepo, events, 5)

	mediaItemRepo = libfakes.NewFakeMediaItemQueryRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItemRepo, streamRepo, searchIdx)

	libAccessRepo = userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		events,
		appuser.WithLibraryAccess(libAccessRepo),
	)

	h = &transcodingHandler{
		svc:        svc,
		libQuery:   libQuery,
		userAccess: userAccess,
	}
	return
}

func TestTranscodingGetTranscodeStatus(t *testing.T) {
	h, transcoder, _, _ := newTestTranscodingHandler()
	transcoder.SetProgress("tj-1", &transcoding.TranscodeProgress{
		JobID:   "tj-1",
		Percent: 42.5,
		FPS:     30.0,
		Speed:   1.5,
	})

	// Claims are required: reading a job's progress is not a public operation,
	// and this call used to slip through only because a missing access port
	// short-circuited the whole check before it ever looked at the caller.
	resp, err := h.GetTranscodeStatus(ctxWithClaims("admin-1", "profile-1", true), &eveyv1.GetTranscodeStatusRequest{JobId: "tj-1"})
	if err != nil {
		t.Fatalf("GetTranscodeStatus: %v", err)
	}
	if resp.Id != "tj-1" {
		t.Errorf("expected job id 'tj-1', got %q", resp.Id)
	}
	if resp.ProgressPercent != 42.5 {
		t.Errorf("expected percent 42.5, got %f", resp.ProgressPercent)
	}
	if resp.Fps != 30.0 {
		t.Errorf("expected fps 30.0, got %f", resp.Fps)
	}
	if resp.Speed != 1.5 {
		t.Errorf("expected speed 1.5, got %f", resp.Speed)
	}
}

func TestTranscodingListActiveTranscodes(t *testing.T) {
	h, _, jobRepo, _ := newTestTranscodingHandler()
	jobRepo.Create(context.Background(), &transcoding.TranscodeJob{
		ID:          "tj-1",
		MediaItemID: "mi-1",
		Status:      transcoding.Running,
		OutputCodec: "h264",
	})
	jobRepo.Create(context.Background(), &transcoding.TranscodeJob{
		ID:          "tj-2",
		MediaItemID: "mi-2",
		Status:      transcoding.Running,
		OutputCodec: "hevc",
	})
	jobRepo.Create(context.Background(), &transcoding.TranscodeJob{
		ID:          "tj-3",
		MediaItemID: "mi-3",
		Status:      transcoding.Completed,
		OutputCodec: "h264",
	})

	resp, err := h.ListActiveTranscodes(context.Background(), &eveyv1.ListActiveTranscodesRequest{})
	if err != nil {
		t.Fatalf("ListActiveTranscodes: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("expected 2 active jobs, got %d", len(resp.Jobs))
	}
}

func TestTranscodingListActiveTranscodesEmpty(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	resp, err := h.ListActiveTranscodes(context.Background(), &eveyv1.ListActiveTranscodesRequest{})
	if err != nil {
		t.Fatalf("ListActiveTranscodes: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(resp.Jobs))
	}
}

func TestTranscodingListProfiles(t *testing.T) {
	h, _, _, profileRepo := newTestTranscodingHandler()
	profileRepo.Create(context.Background(), &transcoding.TranscodeProfile{
		ID:           "tp-1",
		Name:         "720p",
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "mp4",
		MaxWidth:     1280,
		MaxHeight:    720,
		VideoBitrate: 4000000,
	})

	resp, err := h.ListTranscodeProfiles(context.Background(), &eveyv1.ListTranscodeProfilesRequest{})
	if err != nil {
		t.Fatalf("ListTranscodeProfiles: %v", err)
	}
	if len(resp.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(resp.Profiles))
	}
	if resp.Profiles[0].Name != "720p" {
		t.Errorf("expected name '720p', got %q", resp.Profiles[0].Name)
	}
	if resp.Profiles[0].VideoCodec != "h264" {
		t.Errorf("expected codec 'h264', got %q", resp.Profiles[0].VideoCodec)
	}
}

// --- IDOR defense-in-depth on Stop/GetStatus ---

func seedIDORTestFixture(t *testing.T) (
	h *transcodingHandler,
	transcoder *tcfakes.FakeTranscoder,
	jobRepo *tcfakes.FakeTranscodeJobRepository,
) {
	t.Helper()
	handler, trans, jRepo, miRepo, laRepo := newTranscodingHandlerWithAccess()
	miRepo.Seed(&library.MediaItem{
		ID:        "media-1",
		LibraryID: "lib-restricted",
		Title:     "Hidden Movie",
		Type:      "movie",
	})
	ctx := context.Background()
	_ = jRepo.Create(ctx, &transcoding.TranscodeJob{
		ID:          "tj-hidden",
		MediaItemID: "media-1",
		Status:      transcoding.Running,
		OutputCodec: "h264",
	})
	// owner-user has the grant; attacker-user does not.
	_ = laRepo.Grant(ctx, "owner-user", "lib-restricted")
	_ = laRepo.Grant(ctx, "attacker-user", "lib-other")
	trans.SetProgress("tj-hidden", &transcoding.TranscodeProgress{JobID: "tj-hidden", Percent: 10})
	return handler, trans, jRepo
}

func TestGetTranscodeStatus_IDOR_NoAccess_ReturnsNotFound(t *testing.T) {
	h, _, _ := seedIDORTestFixture(t)
	ctx := ctxWithClaims("attacker-user", "prof-1", false)

	_, err := h.GetTranscodeStatus(ctx, &eveyv1.GetTranscodeStatusRequest{JobId: "tj-hidden"})
	if err == nil {
		t.Fatal("expected NotFound for attacker without library access")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound (leaking job existence)", status.Code(err))
	}
}

func TestGetTranscodeStatus_IDOR_Owner_Succeeds(t *testing.T) {
	h, _, _ := seedIDORTestFixture(t)
	ctx := ctxWithClaims("owner-user", "prof-1", false)

	resp, err := h.GetTranscodeStatus(ctx, &eveyv1.GetTranscodeStatusRequest{JobId: "tj-hidden"})
	if err != nil {
		t.Fatalf("owner should have access: %v", err)
	}
	if resp.Id != "tj-hidden" {
		t.Errorf("id = %q, want tj-hidden", resp.Id)
	}
}

func TestGetTranscodeStatus_IDOR_Admin_Bypasses(t *testing.T) {
	h, _, _ := seedIDORTestFixture(t)
	ctx := ctxWithClaims("admin-user", "prof-1", true)

	resp, err := h.GetTranscodeStatus(ctx, &eveyv1.GetTranscodeStatusRequest{JobId: "tj-hidden"})
	if err != nil {
		t.Fatalf("admin should bypass check: %v", err)
	}
	if resp.Id != "tj-hidden" {
		t.Errorf("id = %q, want tj-hidden", resp.Id)
	}
}

func TestStopTranscode_IDOR_NoAccess_ReturnsNotFound(t *testing.T) {
	h, _, _ := seedIDORTestFixture(t)
	ctx := ctxWithClaims("attacker-user", "prof-1", false)

	_, err := h.StopTranscode(ctx, &eveyv1.StopTranscodeRequest{JobId: "tj-hidden"})
	if err == nil {
		t.Fatal("expected NotFound for attacker without library access")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestStopTranscode_IDOR_UnknownJob_ReturnsNotFound(t *testing.T) {
	h, _, _ := seedIDORTestFixture(t)
	ctx := ctxWithClaims("owner-user", "prof-1", false)

	_, err := h.StopTranscode(ctx, &eveyv1.StopTranscodeRequest{JobId: "tj-does-not-exist"})
	if err == nil {
		t.Fatal("expected NotFound for unknown job")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestCheckJobAccess_EmptyJobID_InvalidArgument(t *testing.T) {
	h, _, _ := seedIDORTestFixture(t)
	ctx := ctxWithClaims("owner-user", "prof-1", false)

	err := h.checkJobAccess(ctx, "")
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// A missing access port must refuse a normal user, but it must NOT refuse an
// admin: whether the port is wired says nothing about who is calling, and a
// wiring bug that answers an admin with a denial is the mirror image of the
// fail-open this guard replaced.
func TestTranscodingCheckJobAccess_UnwiredUserAccess(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	h.userAccess = nil

	if err := h.checkJobAccess(ctxWithClaims("admin-1", "profile-1", true), "tj-1"); err != nil {
		t.Errorf("admin must pass on claims alone, got %v", err)
	}

	err := h.checkJobAccess(ctxWithClaims("user-1", "profile-1", false), "tj-1")
	if err == nil {
		t.Fatal("a non-admin must be refused when the access port is unwired")
	}
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition, got %v (%v)", got, err)
	}

	// Unauthenticated stays unauthenticated - not "failed precondition".
	if got := status.Code(h.checkJobAccess(context.Background(), "tj-1")); got != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated for a caller with no claims, got %v", got)
	}
}
