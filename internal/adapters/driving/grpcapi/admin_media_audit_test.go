package grpcapi

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

// Library-wide repairs are the ones that rewrote 98 files in July: the entry
// must carry the options that decide whether a way back exists.
func TestAdminMedia_LibraryRepairsAreAudited(t *testing.T) {
	d, repo := newAuditedMediaHandler(t)
	ctx := ctxWithClaims("adm-1", "prf-adm", true)

	res, err := d.ReindexLibrarySeekIndex(ctx, &eveyv1.ReindexLibrarySeekIndexRequest{
		LibraryId: "lib-1", Mode: "rewrite", DeepVerify: true, SkipBackup: true, BackupWindow: 3, ResyncDamaged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetQueued() != -1 { //nolint:staticcheck // SA1019: the deprecated field must keep its sentinel for old clients
		t.Errorf("queued = %d, want the -1 poll sentinel old clients read", res.GetQueued()) //nolint:staticcheck // SA1019: same
	}
	waitJobDone(t, d)
	got := auditEntries(t, repo, "reindex_library_seek_index")
	if len(got) != 1 {
		t.Fatalf("reindex library: %d audit entries, want 1", len(got))
	}
	if got[0].ResourceType != "library" || got[0].ResourceID == nil || *got[0].ResourceID != "lib-1" {
		t.Errorf("entry = %+v, want the library id", got[0])
	}
	det := got[0].Details
	if det["mode"] != "rewrite" || det["deep_verify"] != true || det["skip_backup"] != true || det["resync_damaged"] != true {
		t.Errorf("details = %+v", det)
	}
	if fmt.Sprint(det["backup_window"]) != "3" {
		t.Errorf("backup_window = %v, want 3", det["backup_window"])
	}

	if _, err := d.RollbackLibrarySeekIndex(ctx, &eveyv1.RollbackLibrarySeekIndexRequest{LibraryId: "lib-1"}); err != nil {
		t.Fatal(err)
	}
	waitJobDone(t, d)
	if len(auditEntries(t, repo, "rollback_library_seek_index")) != 1 {
		t.Error("library rollback must be audited")
	}

	if _, err := d.PurgeSeekIndexBackups(ctx, &eveyv1.PurgeSeekIndexBackupsRequest{LibraryId: "lib-1"}); err != nil {
		t.Fatal(err)
	}
	waitJobDone(t, d)
	if len(auditEntries(t, repo, "purge_seek_index_backups")) != 1 {
		t.Error("purging every backup is the irreversible step; it must be audited")
	}
}

// The retired bulk remux answers with a verdict, not Unimplemented, so a
// deployed client shows the explanation instead of a generic failure - and
// it touches nothing, so nothing is audited.
func TestAdminMedia_RemuxAudioIsRetired(t *testing.T) {
	d, repo := newAuditedMediaHandler(t)
	ctx := ctxWithClaims("adm-1", "prf-adm", true)

	_, err := d.RemuxAudioMedia(ctx, &eveyv1.RemuxAudioMediaRequest{MediaId: "mi-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("media: code = %v, want FailedPrecondition", status.Code(err))
	}
	_, err = d.RemuxAudioLibrary(ctx, &eveyv1.RemuxAudioLibraryRequest{LibraryId: "lib-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("library: code = %v, want FailedPrecondition", status.Code(err))
	}
	if n, _ := repo.List(context.Background(), activity.AuditLogFilter{Limit: 10}); len(n) != 0 {
		t.Errorf("retired RPCs audited %d entries", len(n))
	}
}

// The container fix needs a relocator; without one the RPC says "not
// configured" rather than pretending it queued anything.
func TestAdminMedia_FixContainerWithoutRelocator(t *testing.T) {
	d, repo := newAuditedMediaHandler(t)
	_, err := d.FixContainerMedia(ctxWithClaims("adm-1", "prf-adm", true), &eveyv1.FixContainerMediaRequest{MediaId: "mi-1"})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("code = %v, want Unimplemented", status.Code(err))
	}
	if n := len(auditEntries(t, repo, "fix_container_media")); n != 0 {
		t.Errorf("refused fix audited %d times", n)
	}
}
