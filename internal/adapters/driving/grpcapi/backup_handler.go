package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	domainbackup "github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type backupHandler struct {
	eveyv1.UnimplementedBackupServiceServer
	svc *appbackup.Service
}

func (h *backupHandler) CreateBackup(ctx context.Context, _ *eveyv1.CreateBackupRequest) (*eveyv1.BackupManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.backup.CreateBackup")
	defer span.End()
	m, err := h.svc.CreateBackup(ctx)
	if err != nil {
		return nil, internalError("create backup", err)
	}
	return manifestToProto(m), nil
}

func (h *backupHandler) ListBackups(ctx context.Context, _ *eveyv1.ListBackupsRequest) (*eveyv1.ListBackupsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.backup.ListBackups")
	defer span.End()
	list, err := h.svc.ListBackups(ctx)
	if err != nil {
		return nil, internalError("list backups", err)
	}
	resp := &eveyv1.ListBackupsResponse{}
	for _, m := range list {
		resp.Backups = append(resp.Backups, manifestToProto(m))
	}
	return resp, nil
}

func (h *backupHandler) DeleteBackup(ctx context.Context, req *eveyv1.DeleteBackupRequest) (*eveyv1.DeleteBackupResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.backup.DeleteBackup")
	defer span.End()
	if err := h.svc.DeleteBackup(ctx, req.Id); err != nil {
		// Only a genuine missing backup is NotFound; a store/IO failure is
		// internal and must not be mislabelled NotFound nor leak the raw error.
		if errors.Is(err, domainbackup.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "backup not found")
		}
		return nil, internalError("delete backup", err)
	}
	return &eveyv1.DeleteBackupResponse{}, nil
}

func manifestToProto(m *domainbackup.BackupManifest) *eveyv1.BackupManifest {
	return &eveyv1.BackupManifest{
		Id:        m.ID,
		CreatedAt: timestamppb.New(m.CreatedAt),
		Version:   m.Version,
		Tables:    m.Tables,
		Size:      m.Size,
	}
}
