package postgres

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// IntroFingerprintCacheRepository persists pHash fingerprints in BYTEA
// columns: each uint64 / int64 is encoded little-endian, 8 bytes per
// value. The packed format is ~3-5x smaller than JSON for hash slices and
// trivially decodable since the lengths are deduced from len(buf)/8.
type IntroFingerprintCacheRepository struct {
	db *sql.DB
}

func NewIntroFingerprintCacheRepository(db *sql.DB) *IntroFingerprintCacheRepository {
	return &IntroFingerprintCacheRepository{db: db}
}

func (r *IntroFingerprintCacheRepository) Get(ctx context.Context, mediaItemID string) (*library.IntroFingerprintCacheEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_fingerprint_cache.Get")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var (
		entry    = &library.IntroFingerprintCacheEntry{MediaItemID: mediaItemID}
		tilesRaw []byte
		tsRaw    []byte
	)
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT file_fingerprint, phash_tiles, phash_timestamps, algo_version, extracted_at
		 FROM intro_fingerprint_cache WHERE media_item_id = $1`, mediaItemID,
	).Scan(&entry.FileFingerprint, &tilesRaw, &tsRaw, &entry.AlgoVersion, &entry.ExtractedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	tiles, err := decodeUint64Slice(tilesRaw)
	if err != nil {
		return nil, fmt.Errorf("decode phash_tiles: %w", err)
	}
	timestamps, err := decodeInt64Slice(tsRaw)
	if err != nil {
		return nil, fmt.Errorf("decode phash_timestamps: %w", err)
	}
	if len(tiles) != len(timestamps) {
		return nil, fmt.Errorf("phash_tiles len %d != phash_timestamps len %d", len(tiles), len(timestamps))
	}
	entry.PhashTiles = tiles
	entry.PhashTimestamps = timestamps
	return entry, nil
}

func (r *IntroFingerprintCacheRepository) Upsert(ctx context.Context, entry *library.IntroFingerprintCacheEntry) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_fingerprint_cache.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", entry.MediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if len(entry.PhashTiles) != len(entry.PhashTimestamps) {
		return fmt.Errorf("phash_tiles len %d != phash_timestamps len %d", len(entry.PhashTiles), len(entry.PhashTimestamps))
	}
	tilesRaw := encodeUint64Slice(entry.PhashTiles)
	tsRaw := encodeInt64Slice(entry.PhashTimestamps)

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO intro_fingerprint_cache
		   (media_item_id, file_fingerprint, phash_tiles, phash_timestamps, algo_version, extracted_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   file_fingerprint = EXCLUDED.file_fingerprint,
		   phash_tiles      = EXCLUDED.phash_tiles,
		   phash_timestamps = EXCLUDED.phash_timestamps,
		   algo_version     = EXCLUDED.algo_version,
		   extracted_at     = NOW()`,
		entry.MediaItemID, entry.FileFingerprint, tilesRaw, tsRaw, entry.AlgoVersion,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *IntroFingerprintCacheRepository) Delete(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_fingerprint_cache.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM intro_fingerprint_cache WHERE media_item_id = $1`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *IntroFingerprintCacheRepository) DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_fingerprint_cache.DeleteByAlgoVersionBelow")
	defer span.End()
	span.SetAttributes(attribute.Int("algo_version.below", int(version)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM intro_fingerprint_cache WHERE algo_version < $1`, version)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func encodeUint64Slice(values []uint64) []byte {
	out := make([]byte, 8*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint64(out[i*8:], v)
	}
	return out
}

func encodeInt64Slice(values []int64) []byte {
	out := make([]byte, 8*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint64(out[i*8:], uint64(v))
	}
	return out
}

func decodeUint64Slice(buf []byte) ([]uint64, error) {
	if len(buf)%8 != 0 {
		return nil, fmt.Errorf("buffer len %d not multiple of 8", len(buf))
	}
	out := make([]uint64, len(buf)/8)
	for i := range out {
		out[i] = binary.LittleEndian.Uint64(buf[i*8:])
	}
	return out, nil
}

func decodeInt64Slice(buf []byte) ([]int64, error) {
	if len(buf)%8 != 0 {
		return nil, fmt.Errorf("buffer len %d not multiple of 8", len(buf))
	}
	out := make([]int64, len(buf)/8)
	for i := range out {
		out[i] = int64(binary.LittleEndian.Uint64(buf[i*8:]))
	}
	return out, nil
}

var _ library.IntroFingerprintCacheRepository = (*IntroFingerprintCacheRepository)(nil)
