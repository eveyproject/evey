package grpcapi

import (
	"context"
	"log/slog"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	libraryfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
)

func newTestScheduler() (*applibrary.Scheduler, *libraryfakes.FakeScanScheduleRepository) {
	schedRepo := libraryfakes.NewFakeScanScheduleRepository()
	scheduler := applibrary.NewScheduler(schedRepo, &fakeLibraryScanner{}, slog.Default())
	return scheduler, schedRepo
}

func TestAdminListScheduledTasks_Empty(t *testing.T) {
	scheduler, _ := newTestScheduler()
	d := &adminTaskHandler{scheduler: scheduler}

	resp, err := d.ListScheduledTasks(context.Background(), &eveyv1.ListScheduledTasksRequest{})
	if err != nil {
		t.Fatalf("ListScheduledTasks: %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(resp.Tasks))
	}
}

func TestAdminListScheduledTasks_WithData(t *testing.T) {
	scheduler, schedRepo := newTestScheduler()
	d := &adminTaskHandler{scheduler: scheduler}

	schedRepo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 2 * * *",
		Enabled:        true,
	})
	schedRepo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-2",
		CronExpression: "0 3 * * 0",
		Enabled:        false,
	})

	resp, err := d.ListScheduledTasks(context.Background(), &eveyv1.ListScheduledTasksRequest{})
	if err != nil {
		t.Fatalf("ListScheduledTasks: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(resp.Tasks))
	}
}

func TestAdminUpdateScheduledTask_Success(t *testing.T) {
	scheduler, schedRepo := newTestScheduler()
	d := &adminTaskHandler{scheduler: scheduler}

	schedRepo.Upsert(context.Background(), &domainlibrary.ScanSchedule{
		LibraryID:      "lib-1",
		CronExpression: "0 2 * * *",
		Enabled:        true,
	})

	newCron := "0 4 * * *"
	enabled := false
	resp, err := d.UpdateScheduledTask(context.Background(), &eveyv1.UpdateScheduledTaskRequest{
		Id:             "lib-1",
		CronExpression: &newCron,
		Enabled:        &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateScheduledTask: %v", err)
	}
	if resp.CronExpression != "0 4 * * *" {
		t.Errorf("expected cron %q, got %q", "0 4 * * *", resp.CronExpression)
	}
	if resp.Enabled {
		t.Error("expected enabled=false")
	}
}

func TestAdminUpdateScheduledTask_NotFound(t *testing.T) {
	scheduler, _ := newTestScheduler()
	d := &adminTaskHandler{scheduler: scheduler}

	_, err := d.UpdateScheduledTask(context.Background(), &eveyv1.UpdateScheduledTaskRequest{
		Id: "nonexistent",
	})
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestAdminRunScheduledTask(t *testing.T) {
	scanner := &fakeLibraryScanner{}
	d := &adminTaskHandler{libScanner: scanner}

	resp, err := d.RunScheduledTask(context.Background(), &eveyv1.RunScheduledTaskRequest{
		Id: "lib-1",
	})
	if err != nil {
		t.Fatalf("RunScheduledTask: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestAdminListScheduledTasks_NilScheduler(t *testing.T) {
	d := &adminTaskHandler{}
	resp, err := d.ListScheduledTasks(context.Background(), &eveyv1.ListScheduledTasksRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("expected empty tasks, got %d", len(resp.Tasks))
	}
}

func TestAdminRunScheduledTask_NilScanner(t *testing.T) {
	d := &adminTaskHandler{}
	_, err := d.RunScheduledTask(context.Background(), &eveyv1.RunScheduledTaskRequest{Id: "lib-1"})
	if err == nil {
		t.Error("expected error for nil scanner")
	}
}
