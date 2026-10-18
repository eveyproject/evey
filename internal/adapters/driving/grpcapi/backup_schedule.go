package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	domainbackup "github.com/eveyproject/evey/internal/domain/backup"
	"github.com/eveyproject/evey/internal/pkg/cron"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type backupScheduleHandler struct {
	eveyv1.UnimplementedBackupScheduleServiceServer
	scheduler  *appbackup.Scheduler
	translator *eveyi18n.Translator
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
		if retentionDays <= 0 {
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.admin.backup_retention_positive")
		}
		if _, cronErr := cron.NextTime(cronExpr, time.Now()); cronErr != nil {
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.library.cron_invalid")
		}
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
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.admin.backup_running")
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, internalError(ctx, "run backup now", err)
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
