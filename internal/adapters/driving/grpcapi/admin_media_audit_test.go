package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appactivity "github.com/evey-project/evey/internal/app/activity"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/activity"
	activityfakes "github.com/evey-project/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// Repairs rewrite media bytes and delete their only way back: the admin audit
// log must say WHO asked for WHAT on WHICH file. Until now nothing did.
func newAuditedMediaHandler(t *testing.T) (*adminMediaHandler, *activityfakes.FakeAuditLogRepository) {
	t.Helper()
	auditRepo := activityfakes.NewFakeAuditLogRepository()
	act := appactivity.NewCommandService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		auditRepo,
		sharedfakes.NewFakeEventBus(),
	)
	svc := appmaintenance.NewSeekIndexService(
		fakeReindexer{},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}},
		nil,
	)
	return &adminMediaHandler{seekIndex: svc, activitySvc: act}, auditRepo
}

func auditEntries(t *testing.T, repo *activityfakes.FakeAuditLogRepository, action string) []*activity.AuditLogEntry {
	t.Helper()
	got, err := repo.List(context.Background(), activity.AuditLogFilter{Action: &action, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAdminMedia_RepairsAreAudited(t *testing.T) {
	d, repo := newAuditedMediaHandler(t)
	ctx := ctxWithClaims("adm-1", "prf-adm", true)

	if _, err := d.DeleteSeekIndexBackup(ctx, &eveyv1.DeleteSeekIndexBackupRequest{MediaId: "mi-1"}); err != nil {
		t.Fatal(err)
	}
	got := auditEntries(t, repo, "delete_seek_index_backup")
	if len(got) != 1 {
		t.Fatalf("delete backup: %d audit entries, want 1", len(got))
	}
	if got[0].UserID != "adm-1" || got[0].ResourceType != "media" || got[0].ResourceID == nil || *got[0].ResourceID != "mi-1" {
		t.Errorf("entry = %+v, want the admin and the media id", got[0])
	}

	if _, err := d.ReindexMediaSeekIndex(ctx, &eveyv1.ReindexMediaSeekIndexRequest{MediaId: "mi-1", Mode: "rewrite", SkipBackup: true}); err != nil {
		t.Fatal(err)
	}
	waitJobDone(t, d)
	got = auditEntries(t, repo, "reindex_media_seek_index")
	if len(got) != 1 {
		t.Fatalf("reindex: %d audit entries, want 1", len(got))
	}
	if got[0].Details["skip_backup"] != true || got[0].Details["mode"] != "rewrite" {
		t.Errorf("the options that decide whether a way back exists must be in the entry: %+v", got[0].Details)
	}

	if _, err := d.RollbackMediaSeekIndex(ctx, &eveyv1.RollbackMediaSeekIndexRequest{MediaId: "mi-1"}); err != nil {
		t.Fatal(err)
	}
	waitJobDone(t, d)
	if len(auditEntries(t, repo, "rollback_media_seek_index")) != 1 {
		t.Error("rollback must be audited")
	}
}

// A refused repair (unknown media, read-only volume...) leaves no audit entry:
// the log records what happened, not what was attempted.
func TestAdminMedia_RefusedRepairIsNotAudited(t *testing.T) {
	d, repo := newAuditedMediaHandler(t)
	// No seek-index service at all.
	d.seekIndex = nil
	_, err := d.DeleteSeekIndexBackup(ctxWithClaims("adm-1", "prf-adm", true), &eveyv1.DeleteSeekIndexBackupRequest{MediaId: "mi-1"})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if n := len(auditEntries(t, repo, "delete_seek_index_backup")); n != 0 {
		t.Errorf("refused call audited %d times", n)
	}
}
