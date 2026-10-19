package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/domain/ratings"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// parentalRefreshBatch is one round of RefreshParentalRatings.
const parentalRefreshBatch = 500

// RefreshParentalRatings recomputes the parental columns of every
// classification row not computed under the current rating tables (migration
// 000179): all of them once after the migration, all of them again after a
// table edit, none otherwise. Until a row is refreshed a kids profile does not
// see the item, so this runs at startup. Returns the rows rewritten.
func (r *MediaItemRepository) RefreshParentalRatings(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediaitem.RefreshParentalRatings")
	defer span.End()
	total, after := 0, ""
	for {
		n, last, selected, err := r.refreshParentalBatch(ctx, after)
		total += n
		if err != nil || selected < parentalRefreshBatch {
			return total, err
		}
		after = last
	}
}

// parentalRow is one classification row's parental columns, recomputed.
type parentalRow struct {
	id, key string
	age     int64
}

// refreshParentalBatch rewrites the next stale rows after the given id and
// returns how many it rewrote, the last id it read and how many it read.
func (r *MediaItemRepository) refreshParentalBatch(ctx context.Context, after string) (rewritten int, last string, selected int, err error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	db := txFromCtx(ctx, r.db)
	fresh, last, selected, err := readStaleParentalRows(ctx, db, after)
	if err != nil || len(fresh) == 0 {
		return 0, last, selected, err
	}
	ids, ages, keys := make([]string, len(fresh)), make([]int64, len(fresh)), make([]string, len(fresh))
	for i, row := range fresh {
		ids[i], ages[i], keys[i] = row.id, row.age, row.key
	}
	if _, err = db.ExecContext(ctx, `UPDATE media_classifications c
		SET cert_min_age = v.age, rating_key = v.key, parental_rev = $4
		FROM unnest($1::text[], $2::int[], $3::text[]) AS v(id, age, key)
		WHERE c.media_item_id = v.id`,
		pq.Array(ids), pq.Array(ages), pq.Array(keys), ratings.TablesRevision); err != nil {
		return 0, "", 0, fmt.Errorf("update parental rows: %w", err)
	}
	return len(fresh), last, selected, nil
}

// readStaleParentalRows reads the next batch of rows not computed under the
// current tables and recomputes them, with the last id read and the count.
func readStaleParentalRows(ctx context.Context, db dbExecutor, after string) ([]parentalRow, string, int, error) {
	rows, err := db.QueryContext(ctx, `SELECT media_item_id, official_rating, certifications_by_country
		FROM media_classifications WHERE parental_rev IS DISTINCT FROM $1 AND media_item_id > $2
		ORDER BY media_item_id LIMIT $3`,
		ratings.TablesRevision, after, parentalRefreshBatch)
	if err != nil {
		return nil, "", 0, fmt.Errorf("select stale parental rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []parentalRow
	last, selected := "", 0
	for rows.Next() {
		var id, official string
		var certJSON []byte
		if scanErr := rows.Scan(&id, &official, &certJSON); scanErr != nil {
			return nil, "", 0, fmt.Errorf("scan stale parental row: %w", scanErr)
		}
		last, selected = id, selected+1
		var certs map[string]string
		if json.Unmarshal(certJSON, &certs) != nil {
			// Left as is: an unreadable certification set is unknown, and
			// unknown stays hidden from kids.
			continue
		}
		out = append(out, parentalRow{id: id, age: int64(ratings.CertificationsMinAge(certs)), key: ratings.RatingKey(official)})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", 0, fmt.Errorf("read stale parental rows: %w", rowsErr)
	}
	return out, last, selected, nil
}
