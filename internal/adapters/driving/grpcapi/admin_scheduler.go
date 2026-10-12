package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type adminTaskHandler struct {
	eveyv1.UnimplementedAdminTaskServiceServer
	scheduler  adminScheduler
	libScanner adminLibraryScanner
	serverCtx  context.Context
	translator *eveyi18n.Translator
}

func (d *adminTaskHandler) ListScheduledTasks(ctx context.Context, _ *eveyv1.ListScheduledTasksRequest) (*eveyv1.ListScheduledTasksResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.ListScheduledTasks")
	defer span.End()
	if d.scheduler == nil {
		return &eveyv1.ListScheduledTasksResponse{}, nil
	}
	schedules, err := d.scheduler.ListSchedules(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list scheduled tasks", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListScheduledTasksResponse{}
	for _, s := range schedules {
		task := &eveyv1.ScheduledTask{
			Id:             string(s.LibraryID),
			Name:           "Library scan: " + string(s.LibraryID),
			CronExpression: s.CronExpression,
			Enabled:        s.Enabled,
		}
		if s.LastRunAt != nil {
			task.LastRunAt = timestamppb.New(*s.LastRunAt)
		}
		if s.NextRunAt != nil {
			task.NextRunAt = timestamppb.New(*s.NextRunAt)
		}
		resp.Tasks = append(resp.Tasks, task)
	}
	return resp, nil
}

func (d *adminTaskHandler) UpdateScheduledTask(ctx context.Context, req *eveyv1.UpdateScheduledTaskRequest) (*eveyv1.ScheduledTask, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.UpdateScheduledTask")
	defer span.End()
	if d.scheduler == nil {
		span.SetStatus(otelcodes.Error, "scheduler not available")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.scheduler_not_available")
	}
	existing, err := d.scheduler.ListSchedules(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "list schedules", err)
	}
	var current *domainlibrary.ScanSchedule
	for _, s := range existing {
		if string(s.LibraryID) == req.Id {
			current = s
			break
		}
	}
	if current == nil {
		span.SetStatus(otelcodes.Error, "scheduled task not found")
		return nil, d.translator.TranslatedError(ctx, codes.NotFound, "errors.admin.scheduled_task_not_found")
	}
	cron := current.CronExpression
	if req.CronExpression != nil {
		cron = *req.CronExpression
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updated, err := d.scheduler.SetSchedule(ctx, req.Id, cron, enabled)
	if err != nil {
		slog.ErrorContext(ctx, "update scheduled task", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	task := &eveyv1.ScheduledTask{
		Id:             string(updated.LibraryID),
		Name:           "Library scan: " + string(updated.LibraryID),
		CronExpression: updated.CronExpression,
		Enabled:        updated.Enabled,
	}
	if updated.NextRunAt != nil {
		task.NextRunAt = timestamppb.New(*updated.NextRunAt)
	}
	return task, nil
}

func (d *adminTaskHandler) RunScheduledTask(ctx context.Context, req *eveyv1.RunScheduledTaskRequest) (*eveyv1.RunScheduledTaskResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.admin.RunScheduledTask")
	defer span.End()
	if d.libScanner == nil {
		span.SetStatus(otelcodes.Error, "scanner not available")
		return nil, d.translator.TranslatedError(ctx, codes.Unimplemented, "errors.admin.scanner_not_available")
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic in RunScheduledTask", slog.Any("recover", r), slog.String("libraryID", req.Id))
			}
		}()
		added, updated, err := d.libScanner.ScanLibrary(d.serverCtx, req.Id)
		if err != nil {
			slog.ErrorContext(ctx, "manual scan failed", slog.String("libraryID", req.Id), slog.Any("err", err))
		} else {
			slog.Info("manual scan done", slog.String("libraryID", req.Id), slog.Int("added", added), slog.Int("updated", updated))
		}
	}()
	return &eveyv1.RunScheduledTaskResponse{}, nil
}
