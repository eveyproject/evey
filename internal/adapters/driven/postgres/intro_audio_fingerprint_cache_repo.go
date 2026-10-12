package postgres

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// IntroAudioFingerprintCacheRepository persists Chromaprint fingerprints in a
// BYTEA column: each uint32 is encoded little-endian, 4 bytes per value.
// The packed format is ~4× smaller than JSON for hash slices and trivially
// decodable since the element count is deduced from len(buf)/4.
type IntroAudioFingerprintCacheRepository struct {
	db *sql.DB
}

func NewIntroAudioFingerprintCacheRepository(db *sql.DB) *IntroAudioFingerprintCacheRepository {
	return &IntroAudioFingerprintCacheRepository{db: db}
}

func (r *IntroAudioFingerprintCacheRepository) Get(ctx context.Context, mediaItemID string) (*library.IntroAudioFingerprintCacheEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_audio_fingerprint_cache.Get")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var (
		entry = &library.IntroAudioFingerprintCacheEntry{MediaItemID: mediaItemID}
		fpRaw []byte
	)
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT file_fingerprint, window_sec, fingerprint, algo_version, extracted_at
		 FROM intro_audio_fingerprint_cache WHERE media_item_id = $1`, mediaItemID,
	).Scan(&entry.FileFingerprint, &entry.WindowSec, &fpRaw, &entry.AlgoVersion, &entry.ExtractedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	fp, err := decodeUint32Slice(fpRaw)
	if err != nil {
		return nil, fmt.Errorf("decode fingerprint: %w", err)
	}
	entry.Fingerprint = fp
	return entry, nil
}

// GetBatch fetches every cached entry for the given ids in ONE query; absent
// ids are simply missing from the map. Drives the season-cluster pass (one
// round trip per season instead of one per episode).
func (r *IntroAudioFingerprintCacheRepository) GetBatch(ctx context.Context, mediaItemIDs []string) (map[string]*library.IntroAudioFingerprintCacheEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_audio_fingerprint_cache.GetBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("ids.count", len(mediaItemIDs)))
	if len(mediaItemIDs) == 0 {
		return map[string]*library.IntroAudioFingerprintCacheEntry{}, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT media_item_id, file_fingerprint, window_sec, fingerprint, algo_version, extracted_at
		 FROM intro_audio_fingerprint_cache WHERE media_item_id = ANY($1)`, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]*library.IntroAudioFingerprintCacheEntry, len(mediaItemIDs))
	for rows.Next() {
		entry := &library.IntroAudioFingerprintCacheEntry{}
		var fpRaw []byte
		if err := rows.Scan(&entry.MediaItemID, &entry.FileFingerprint, &entry.WindowSec, &fpRaw, &entry.AlgoVersion, &entry.ExtractedAt); err != nil {
			span.RecordError(err)
			return nil, err
		}
		fp, decErr := decodeUint32Slice(fpRaw)
		if decErr != nil {
			return nil, fmt.Errorf("decode fingerprint for %s: %w", entry.MediaItemID, decErr)
		}
		entry.Fingerprint = fp
		out[entry.MediaItemID] = entry
	}
	return out, rows.Err()
}

func (r *IntroAudioFingerprintCacheRepository) Upsert(ctx context.Context, entry *library.IntroAudioFingerprintCacheEntry) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_audio_fingerprint_cache.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", entry.MediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	fpRaw := encodeUint32Slice(entry.Fingerprint)

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO intro_audio_fingerprint_cache
		   (media_item_id, file_fingerprint, window_sec, fingerprint, algo_version, extracted_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (media_item_id) DO UPDATE SET
		   file_fingerprint = EXCLUDED.file_fingerprint,
		   window_sec       = EXCLUDED.window_sec,
		   fingerprint      = EXCLUDED.fingerprint,
		   algo_version     = EXCLUDED.algo_version,
		   extracted_at     = NOW()`,
		entry.MediaItemID, entry.FileFingerprint, entry.WindowSec, fpRaw, entry.AlgoVersion,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *IntroAudioFingerprintCacheRepository) Delete(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_audio_fingerprint_cache.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM intro_audio_fingerprint_cache WHERE media_item_id = $1`, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *IntroAudioFingerprintCacheRepository) DeleteByAlgoVersionBelow(ctx context.Context, version int16) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_audio_fingerprint_cache.DeleteByAlgoVersionBelow")
	defer span.End()
	span.SetAttributes(attribute.Int("algo_version.below", int(version)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM intro_audio_fingerprint_cache WHERE algo_version < $1`, version)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func encodeUint32Slice(values []uint32) []byte {
	out := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(out[i*4:], v)
	}
	return out
}

func decodeUint32Slice(buf []byte) ([]uint32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("buffer len %d not multiple of 4", len(buf))
	}
	out := make([]uint32, len(buf)/4)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(buf[i*4:])
	}
	return out, nil
}

var _ library.IntroAudioFingerprintCacheRepository = (*IntroAudioFingerprintCacheRepository)(nil)
