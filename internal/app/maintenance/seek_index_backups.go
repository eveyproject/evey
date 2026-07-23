package maintenance

import (
	"context"
	"fmt"
	"log/slog"
)

// seek_index_backups.go holds backup lifecycle: rollback (restore from
// .mkvgobak) and purge/delete of the reindex backups.

// purgeRolledBackup deletes one media file's backup as it leaves the rolling
// window (or at the end-of-job drain) and folds the deletion into the cached
// scan. Best-effort: a failure only leaves that one backup behind,
// reclaimable via the regular purge action.
func (s *SeekIndexService) purgeRolledBackup(ctx context.Context, path string) {
	if err := s.reindexer.DeleteBackup(ctx, path); err != nil {
		s.logger.Warn("seek-index: rolling backup purge failed (backup left in place)",
			slog.String("path", path), slog.Any("err", err))
		return
	}
	s.updateCachedFile(path, func(st *SeekStatus) { st.HasBackup = false })
}

// StartRollbackLibrary kicks a background job that re-scans the library then
// restores every backed-up file from its .mkvgobak.
func (s *SeekIndexService) StartRollbackLibrary(_ context.Context, libraryID string) error {
	return s.startBatch(JobRollback, libraryID, nil,
		func(it SeekStatus) bool { return it.HasBackup },
		func(bg context.Context, path string) (bool, error) {
			if err := s.reindexer.RestoreBackup(bg, path); err != nil {
				return false, err
			}
			// A backup is only ever created when repairing an un-indexed file,
			// so the restored original is back to "no index".
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = false; st.HasBackup = false })
			return false, nil
		}, nil)
}

// DeleteBackupMedia removes one media file's reindex backup, reclaiming the
// duplicated disk space. Synchronous - an unlink is instant, no job needed -
// and allowed while a job runs (it doesn't touch the media file itself).
func (s *SeekIndexService) DeleteBackupMedia(ctx context.Context, mediaID string) error {
	f, err := s.catalog.GetMediaFile(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("seek-index delete-backup: lookup media %s: %w", mediaID, err)
	}
	if f.Path == "" {
		return fmt.Errorf("seek-index delete-backup: media %s has no file", mediaID)
	}
	if err := s.reindexer.DeleteBackup(ctx, f.Path); err != nil {
		return err
	}
	s.updateCachedFile(f.Path, func(st *SeekStatus) { st.HasBackup = false })
	return nil
}

// StartPurgeBackups kicks a background job that re-scans the scope then
// deletes every reindex backup in it - the bulk "the repairs are confirmed
// fine, give me my disk back" action.
func (s *SeekIndexService) StartPurgeBackups(_ context.Context, libraryID string) error {
	return s.startBatch(JobPurge, libraryID, nil,
		func(it SeekStatus) bool { return it.HasBackup },
		func(bg context.Context, path string) (bool, error) {
			if err := s.reindexer.DeleteBackup(bg, path); err != nil {
				return false, err
			}
			s.updateCachedFile(path, func(st *SeekStatus) { st.HasBackup = false })
			return false, nil
		}, nil)
}

// StartRollbackMedia restores one media file from its backup in the background.
func (s *SeekIndexService) StartRollbackMedia(ctx context.Context, mediaID string) error {
	return s.startSingle(ctx, JobRollback, mediaID, func(bg context.Context, path string) (bool, error) {
		if err := s.reindexer.RestoreBackup(bg, path); err != nil {
			return false, err
		}
		s.updateCachedFile(path, func(st *SeekStatus) { st.HasIndex = false; st.HasBackup = false })
		return false, nil
	})
}
