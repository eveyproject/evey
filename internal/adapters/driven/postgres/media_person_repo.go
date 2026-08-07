package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type MediaPersonRepository struct {
	db *sql.DB
}

func NewMediaPersonRepository(db *sql.DB) *MediaPersonRepository {
	return &MediaPersonRepository{db: db}
}

func (r *MediaPersonRepository) Create(ctx context.Context, mp *library.MediaPerson) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_person.Create")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mp.MediaItemID)), attribute.String("person.id", string(mp.PersonID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_people (media_item_id, person_id, role_type, character_name, sort_order)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (media_item_id, person_id, role_type) DO NOTHING`,
		mp.MediaItemID, mp.PersonID, mp.RoleType, mp.CharacterName, mp.SortOrder,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaPersonRepository) CreateBatch(ctx context.Context, mps []*library.MediaPerson) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_person.CreateBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("media_person.count", len(mps)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mps) == 0 {
		return nil
	}
	const cols = 5
	vals := make([]string, 0, len(mps))
	args := make([]any, 0, len(mps)*cols)
	for i, mp := range mps {
		base := i * cols
		vals = append(vals, fmt.Sprintf("($%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5))
		args = append(args, mp.MediaItemID, mp.PersonID, mp.RoleType, mp.CharacterName, mp.SortOrder)
	}
	q := `INSERT INTO media_people (media_item_id, person_id, role_type, character_name, sort_order) VALUES ` +
		strings.Join(vals, ",") +
		` ON CONFLICT (media_item_id, person_id, role_type) DO NOTHING`
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, q, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaPersonRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_person.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_people WHERE media_item_id = $1`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// DeleteByMediaItemIDs is the batch form of DeleteByMediaItemID: one
// statement for a whole level of items instead of one round-trip per item.
func (r *MediaPersonRepository) DeleteByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_person.DeleteByMediaItemIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	if len(mediaItemIDs) == 0 {
		return nil
	}
	ids := make([]string, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		ids[i] = string(id)
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM media_people WHERE media_item_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaPersonRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaPerson, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_person.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT media_item_id, person_id, role_type, character_name, sort_order
		 FROM media_people WHERE media_item_id = $1 ORDER BY sort_order`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaPerson
	for rows.Next() {
		mp := &library.MediaPerson{}
		if err := rows.Scan(&mp.MediaItemID, &mp.PersonID, &mp.RoleType, &mp.CharacterName, &mp.SortOrder); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, mp)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// CastNamesByMediaItems batch-fetches the people names linked to many media
// items in one query, keyed by media item ID. Feeds the searchable "cast"
// attribute on the batch indexing paths (reindex/reconcile/drain), which
// otherwise rebuild documents without cast. Names are deduplicated per item (a
// person credited in several roles yields several media_people rows) and kept
// in sort_order.
func (r *MediaPersonRepository) CastNamesByMediaItems(ctx context.Context, mediaItemIDs []string) (map[string][]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_person.CastNamesByMediaItems")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	if len(mediaItemIDs) == 0 {
		return map[string][]string{}, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mp.media_item_id, pe.name
		 FROM media_people mp
		 JOIN people pe ON pe.id = mp.person_id
		 WHERE mp.media_item_id = ANY($1)
		 ORDER BY mp.media_item_id, mp.sort_order`, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string, len(mediaItemIDs))
	seen := make(map[string]map[string]bool)
	for rows.Next() {
		var itemID, name string
		if err := rows.Scan(&itemID, &name); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if seen[itemID] == nil {
			seen[itemID] = make(map[string]bool)
		}
		if seen[itemID][name] {
			continue
		}
		seen[itemID][name] = true
		result[itemID] = append(result[itemID], name)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ library.MediaPersonRepository = (*MediaPersonRepository)(nil)
