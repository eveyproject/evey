package postgres

// intro_fingerprint_legacy.go - reading the intro fingerprint caches out of
// the database so they can move to their on-disk store, then deleting what
// moved. The server no longer reads or writes these tables otherwise: a row
// here is either one that has not moved yet, or one an older backup brought
// back.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/domain/library"
)

// Page returns up to limit entries with a media item id after afterID, in id
// order, with their original extraction time.
func (r *IntroAudioFingerprintCacheRepository) Page(ctx context.Context, afterID string, limit int) ([]*library.IntroAudioFingerprintCacheEntry, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT media_item_id, file_fingerprint, window_sec, fingerprint, algo_version, extracted_at
		 FROM intro_audio_fingerprint_cache WHERE media_item_id > $1 ORDER BY media_item_id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*library.IntroAudioFingerprintCacheEntry
	for rows.Next() {
		e := &library.IntroAudioFingerprintCacheEntry{}
		var raw []byte
		if err := rows.Scan(&e.MediaItemID, &e.FileFingerprint, &e.WindowSec, &raw, &e.AlgoVersion, &e.ExtractedAt); err != nil {
			return nil, err
		}
		fp, err := decodeUint32Slice(raw)
		if err != nil {
			// Undecodable: an empty entry tells the caller to drop the row
			// without keeping it.
			fp = nil
		}
		e.Fingerprint = fp
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteIDs removes the given entries.
func (r *IntroAudioFingerprintCacheRepository) DeleteIDs(ctx context.Context, ids []string) error {
	return deleteFingerprintRows(ctx, r.db, "intro_audio_fingerprint_cache", ids)
}

// Page returns up to limit entries with a media item id after afterID, in id
// order, with their original extraction time.
func (r *IntroFingerprintCacheRepository) Page(ctx context.Context, afterID string, limit int) ([]*library.IntroFingerprintCacheEntry, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT media_item_id, file_fingerprint, phash_tiles, phash_timestamps, algo_version, extracted_at
		 FROM intro_fingerprint_cache WHERE media_item_id > $1 ORDER BY media_item_id LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*library.IntroFingerprintCacheEntry
	for rows.Next() {
		e := &library.IntroFingerprintCacheEntry{}
		var tilesRaw, tsRaw []byte
		if err := rows.Scan(&e.MediaItemID, &e.FileFingerprint, &tilesRaw, &tsRaw, &e.AlgoVersion, &e.ExtractedAt); err != nil {
			return nil, err
		}
		tiles, errT := decodeUint64Slice(tilesRaw)
		ts, errS := decodeInt64Slice(tsRaw)
		if errT == nil && errS == nil && len(tiles) == len(ts) {
			e.PhashTiles, e.PhashTimestamps = tiles, ts
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteIDs removes the given entries.
func (r *IntroFingerprintCacheRepository) DeleteIDs(ctx context.Context, ids []string) error {
	return deleteFingerprintRows(ctx, r.db, "intro_fingerprint_cache", ids)
}

func deleteFingerprintRows(ctx context.Context, db *sql.DB, table string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	//nolint:gosec // G201: table is one of two constants of this file, never input
	_, err := db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE media_item_id = ANY($1)`, table), pq.Array(ids))
	return err
}

// ExistingMediaItemIDs returns which of ids are still media items.
func ExistingMediaItemIDs(ctx context.Context, db *sql.DB, ids []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT id FROM media_items WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}
