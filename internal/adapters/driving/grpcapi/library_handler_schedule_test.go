package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	libraryfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newScheduleHandler() (*libraryHandler, *libraryfakes.FakeScanScheduleRepository) {
	schedRepo := libraryfakes.NewFakeScanScheduleRepository()
	scheduler := applibrary.NewScheduler(schedRepo, &fakeLibraryScanner{}, slog.Default())
	return &libraryHandler{scheduler: scheduler}, schedRepo
}

func TestLibraryHandler_GetLibrarySchedule_Success(t *testing.T) {
	h, repo := newScheduleHandler()
	last := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	next := time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC)
	require.NoError(t, repo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 2 * * *",
		Enabled:        true,
		LastRunAt:      &last,
		NextRunAt:      &next,
	}))

	resp, err := h.GetLibrarySchedule(adminCtx(), &eveyv1.GetLibraryScheduleRequest{Id: "lib-1"})

	require.NoError(t, err)
	require.NotNil(t, resp.Schedule)
	assert.Equal(t, "lib-1", resp.Schedule.LibraryId)
	assert.Equal(t, "0 2 * * *", resp.Schedule.Cron)
	assert.True(t, resp.Schedule.Enabled)
	require.NotNil(t, resp.Schedule.LastRunAt)
	assert.Equal(t, last.Unix(), resp.Schedule.LastRunAt.Seconds)
	require.NotNil(t, resp.Schedule.NextRunAt)
	assert.Equal(t, next.Unix(), resp.Schedule.NextRunAt.Seconds)
}

func TestLibraryHandler_GetLibrarySchedule_NotFoundReturnsEmpty(t *testing.T) {
	h, _ := newScheduleHandler()

	resp, err := h.GetLibrarySchedule(adminCtx(), &eveyv1.GetLibraryScheduleRequest{Id: "missing"})

	require.NoError(t, err)
	assert.Nil(t, resp.Schedule)
}

func TestLibraryHandler_GetLibrarySchedule_NilScheduler(t *testing.T) {
	h := &libraryHandler{}

	resp, err := h.GetLibrarySchedule(adminCtx(), &eveyv1.GetLibraryScheduleRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.Nil(t, resp.Schedule)
}

func TestLibraryHandler_GetLibrarySchedule_NonAdmin(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.GetLibrarySchedule(userCtx("user-1", "profile-1"), &eveyv1.GetLibraryScheduleRequest{Id: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestLibraryHandler_GetLibrarySchedule_Unauthenticated(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.GetLibrarySchedule(context.Background(), &eveyv1.GetLibraryScheduleRequest{Id: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestLibraryHandler_SetLibrarySchedule_Success(t *testing.T) {
	h, repo := newScheduleHandler()

	resp, err := h.SetLibrarySchedule(adminCtx(), &eveyv1.SetLibraryScheduleRequest{
		Id:      "lib-1",
		Cron:    "0 2 * * *",
		Enabled: true,
	})

	require.NoError(t, err)
	assert.Equal(t, "lib-1", resp.LibraryId)
	assert.Equal(t, "0 2 * * *", resp.Cron)
	assert.True(t, resp.Enabled)

	stored, err := repo.GetByLibraryID(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "0 2 * * *", stored.CronExpression)
	assert.True(t, stored.Enabled)
}

func TestLibraryHandler_SetLibrarySchedule_EmptyCron(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.SetLibrarySchedule(adminCtx(), &eveyv1.SetLibraryScheduleRequest{
		Id:   "lib-1",
		Cron: "",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestLibraryHandler_SetLibrarySchedule_InvalidCron(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.SetLibrarySchedule(adminCtx(), &eveyv1.SetLibraryScheduleRequest{
		Id:   "lib-1",
		Cron: "not-a-cron",
	})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestLibraryHandler_SetLibrarySchedule_RepoError(t *testing.T) {
	h, repo := newScheduleHandler()
	repo.Err = errors.New("boom")

	_, err := h.SetLibrarySchedule(adminCtx(), &eveyv1.SetLibraryScheduleRequest{
		Id:      "lib-1",
		Cron:    "0 2 * * *",
		Enabled: true,
	})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestLibraryHandler_SetLibrarySchedule_NilScheduler(t *testing.T) {
	h := &libraryHandler{}

	_, err := h.SetLibrarySchedule(adminCtx(), &eveyv1.SetLibraryScheduleRequest{
		Id:   "lib-1",
		Cron: "0 2 * * *",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestLibraryHandler_SetLibrarySchedule_NonAdmin(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.SetLibrarySchedule(userCtx("user-1", "profile-1"), &eveyv1.SetLibraryScheduleRequest{
		Id:   "lib-1",
		Cron: "0 2 * * *",
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestLibraryHandler_SetLibrarySchedule_Unauthenticated(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.SetLibrarySchedule(context.Background(), &eveyv1.SetLibraryScheduleRequest{
		Id:   "lib-1",
		Cron: "0 2 * * *",
	})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestLibraryHandler_DeleteLibrarySchedule_Success(t *testing.T) {
	h, repo := newScheduleHandler()
	require.NoError(t, repo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 2 * * *",
		Enabled:        true,
	}))

	resp, err := h.DeleteLibrarySchedule(adminCtx(), &eveyv1.DeleteLibraryScheduleRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.NotNil(t, resp)

	_, err = repo.GetByLibraryID(context.Background(), "lib-1")
	assert.Error(t, err)
}

func TestLibraryHandler_DeleteLibrarySchedule_NilScheduler(t *testing.T) {
	h := &libraryHandler{}

	resp, err := h.DeleteLibrarySchedule(adminCtx(), &eveyv1.DeleteLibraryScheduleRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestLibraryHandler_DeleteLibrarySchedule_RepoErrorSwallowed(t *testing.T) {
	h, repo := newScheduleHandler()
	repo.Err = errors.New("boom")

	resp, err := h.DeleteLibrarySchedule(adminCtx(), &eveyv1.DeleteLibraryScheduleRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestLibraryHandler_DeleteLibrarySchedule_NonAdmin(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.DeleteLibrarySchedule(userCtx("user-1", "profile-1"), &eveyv1.DeleteLibraryScheduleRequest{Id: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestLibraryHandler_DeleteLibrarySchedule_Unauthenticated(t *testing.T) {
	h, _ := newScheduleHandler()

	_, err := h.DeleteLibrarySchedule(context.Background(), &eveyv1.DeleteLibraryScheduleRequest{Id: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestScheduleToProto_NilTimestamps(t *testing.T) {
	pb := scheduleToProto(&domainlibrary.ScanSchedule{
		LibraryID:      "lib-xyz",
		CronExpression: "*/5 * * * *",
		Enabled:        false,
	})

	assert.Equal(t, "lib-xyz", pb.LibraryId)
	assert.Equal(t, "*/5 * * * *", pb.Cron)
	assert.False(t, pb.Enabled)
	assert.Nil(t, pb.LastRunAt)
	assert.Nil(t, pb.NextRunAt)
}

func TestScheduleToProto_WithTimestamps(t *testing.T) {
	last := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	next := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	pb := scheduleToProto(&domainlibrary.ScanSchedule{
		LibraryID:      "lib-abc",
		CronExpression: "0 12 * * *",
		Enabled:        true,
		LastRunAt:      &last,
		NextRunAt:      &next,
	})

	assert.Equal(t, "lib-abc", pb.LibraryId)
	assert.True(t, pb.Enabled)
	require.NotNil(t, pb.LastRunAt)
	assert.Equal(t, last.Unix(), pb.LastRunAt.Seconds)
	require.NotNil(t, pb.NextRunAt)
	assert.Equal(t, next.Unix(), pb.NextRunAt.Seconds)
}
