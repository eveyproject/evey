package grpcapi

import (
	"context"
	"errors"
	"log/slog"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	domainbackup "github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type backupScheduleHandler struct {
	eveyv1.UnimplementedBackupScheduleServiceServer
	scheduler *appbackup.Scheduler
}

func (h *backupScheduleHandler) GetBackupSchedule(ctx context.Context, _ *eveyv1.GetBackupScheduleRequest) (*eveyv1.BackupSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.backup_schedule.GetBackupSchedule")
	defer span.End()
	sched, err := h.scheduler.GetSchedule(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "get backup schedule", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return backupScheduleToProto(sched), nil
}

func (h *backupScheduleHandler) UpdateBackupSchedule(ctx context.Context, req *eveyv1.UpdateBackupScheduleRequest) (*eveyv1.BackupSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.backup_schedule.UpdateBackupSchedule")
	defer span.End()

	// Load current values as defaults for partial updates.
	current, err := h.scheduler.GetSchedule(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "get backup schedule for update", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	cronExpr := current.CronExpression
	if req.CronExpression != nil {
		cronExpr = *req.CronExpression
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	retentionDays := current.RetentionDays
	if req.RetentionDays != nil {
		retentionDays = int(*req.RetentionDays)
	}

	updated, err := h.scheduler.UpdateSchedule(ctx, cronExpr, enabled, retentionDays)
	if err != nil {
		slog.ErrorContext(ctx, "update backup schedule", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "invalid schedule: %v", err)
	}
	return backupScheduleToProto(updated), nil
}

func (h *backupScheduleHandler) RunBackupNow(ctx context.Context, _ *eveyv1.RunBackupNowRequest) (*eveyv1.RunBackupNowResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.backup_schedule.RunBackupNow")
	defer span.End()
	m, err := h.scheduler.RunNow(ctx)
	if err != nil {
		if errors.Is(err, appbackup.ErrBackupInProgress) {
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, status.Error(codes.FailedPrecondition, "a backup is already running")
		}
		slog.ErrorContext(ctx, "run backup now", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "backup failed: %v", err)
	}
	return &eveyv1.RunBackupNowResponse{BackupId: m.ID}, nil
}

func backupScheduleToProto(s *domainbackup.BackupSchedule) *eveyv1.BackupSchedule {
	p := &eveyv1.BackupSchedule{
		CronExpression: s.CronExpression,
		Enabled:        s.Enabled,
		RetentionDays:  int32(s.RetentionDays),
		LastStatus:     s.LastStatus,
		LastError:      s.LastError,
		UpdatedAt:      timestamppb.New(s.UpdatedAt),
	}
	if s.LastRunAt != nil {
		p.LastRunAt = timestamppb.New(*s.LastRunAt)
	}
	if s.NextRunAt != nil {
		p.NextRunAt = timestamppb.New(*s.NextRunAt)
	}
	return p
}
