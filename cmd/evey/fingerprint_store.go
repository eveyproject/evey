package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
)

// fingerprintRoot is where the intro-detection fingerprints live, beside the
// other derived per-media stores of the data directory.
const fingerprintRoot = "/data/fingerprints"

const (
	fingerprintMovePage   = 200
	fingerprintPruneBatch = 1000
	fingerprintPruneEvery = 24 * time.Hour
)

// startFingerprintHousekeeping moves the fingerprints still held by the
// database to the on-disk stores, then sweeps the files of media items that no
// longer exist (the database used to drop them by cascade) at boot and once a
// day. Detached from the boot path: until a fingerprint has moved, a detection
// that needs it recomputes it, which is slow but correct.
func startFingerprintHousekeeping(ctx context.Context, db *sql.DB, r *postgres.Repos, audio *filesystem.AudioFingerprintStore, phash *filesystem.PhashFingerprintStore) {
	go func() {
		moveAudioFingerprints(ctx, r.IntroAudioFingerprintCache, audio)
		movePhashFingerprints(ctx, r.IntroFingerprintCache, phash)
		t := time.NewTicker(fingerprintPruneEvery)
		defer t.Stop()
		for {
			pruneFingerprintOrphans(ctx, db, "audio", audio.MediaItemIDs, audio.Delete)
			pruneFingerprintOrphans(ctx, db, "phash", phash.MediaItemIDs, phash.Delete)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// moveAudioFingerprints copies each row to disk and deletes it from the
// database once written. A file already on disk wins: it is at least as recent
// as a row, which can only come from before the move or from an older backup.
func moveAudioFingerprints(ctx context.Context, repo *postgres.IntroAudioFingerprintCacheRepository, store *filesystem.AudioFingerprintStore) {
	moved, after := 0, ""
	for ctx.Err() == nil {
		page, err := repo.Page(ctx, after, fingerprintMovePage)
		if err != nil {
			slog.WarnContext(ctx, "fingerprints: read audio rows", slog.Any("err", err))
			return
		}
		if len(page) == 0 {
			break
		}
		done := make([]string, 0, len(page))
		for _, e := range page {
			after = e.MediaItemID
			if len(e.Fingerprint) > 0 && !store.Has(e.MediaItemID) {
				if err := store.Upsert(ctx, e); err != nil {
					slog.WarnContext(ctx, "fingerprints: write audio file", slog.String("media_id", e.MediaItemID), slog.Any("err", err))
					continue
				}
				moved++
			}
			done = append(done, e.MediaItemID)
		}
		if err := repo.DeleteIDs(ctx, done); err != nil {
			slog.WarnContext(ctx, "fingerprints: delete moved audio rows", slog.Any("err", err))
			return
		}
	}
	if moved > 0 {
		slog.InfoContext(ctx, "fingerprints: audio moved to disk", slog.Int("count", moved))
	}
}

func movePhashFingerprints(ctx context.Context, repo *postgres.IntroFingerprintCacheRepository, store *filesystem.PhashFingerprintStore) {
	moved, after := 0, ""
	for ctx.Err() == nil {
		page, err := repo.Page(ctx, after, fingerprintMovePage)
		if err != nil {
			slog.WarnContext(ctx, "fingerprints: read phash rows", slog.Any("err", err))
			return
		}
		if len(page) == 0 {
			break
		}
		done := make([]string, 0, len(page))
		for _, e := range page {
			after = e.MediaItemID
			if len(e.PhashTiles) > 0 && !store.Has(e.MediaItemID) {
				if err := store.Upsert(ctx, e); err != nil {
					slog.WarnContext(ctx, "fingerprints: write phash file", slog.String("media_id", e.MediaItemID), slog.Any("err", err))
					continue
				}
				moved++
			}
			done = append(done, e.MediaItemID)
		}
		if err := repo.DeleteIDs(ctx, done); err != nil {
			slog.WarnContext(ctx, "fingerprints: delete moved phash rows", slog.Any("err", err))
			return
		}
	}
	if moved > 0 {
		slog.InfoContext(ctx, "fingerprints: phash moved to disk", slog.Int("count", moved))
	}
}

// pruneFingerprintOrphans removes the files of media items that are gone.
// Any database error stops the sweep without removing anything more: deleting
// on a failed lookup would empty the store.
func pruneFingerprintOrphans(ctx context.Context, db *sql.DB, kind string, list func(context.Context) ([]string, error), remove func(context.Context, string) error) {
	ids, err := list(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "fingerprints: list files", slog.String("kind", kind), slog.Any("err", err))
		}
		return
	}
	removed := 0
	for start := 0; start < len(ids) && ctx.Err() == nil; start += fingerprintPruneBatch {
		batch := ids[start:min(start+fingerprintPruneBatch, len(ids))]
		live, err := postgres.ExistingMediaItemIDs(ctx, db, batch)
		if err != nil {
			slog.WarnContext(ctx, "fingerprints: look up media items", slog.String("kind", kind), slog.Any("err", err))
			return
		}
		for _, id := range batch {
			if _, ok := live[id]; ok {
				continue
			}
			if err := remove(ctx, id); err == nil {
				removed++
			}
		}
	}
	if removed > 0 {
		slog.InfoContext(ctx, "fingerprints: removed files of deleted media", slog.String("kind", kind), slog.Int("count", removed))
	}
}
