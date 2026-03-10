package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type ChapterRepository struct {
	db *sql.DB
}

func NewChapterRepository(db *sql.DB) *ChapterRepository {
	return &ChapterRepository{db: db}
}

func (r *ChapterRepository) ReplaceBatch(ctx context.Context, mediaItemID string, chapters []*library.Chapter) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM chapters WHERE media_item_id = $1`, mediaItemID); err != nil {
		return err
	}

	if len(chapters) == 0 {
		return tx.Commit()
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO chapters (id, media_item_id, title, start_time_ms, end_time_ms, sort_order) VALUES `)
	args := make([]interface{}, 0, len(chapters)*6)
	for i, ch := range chapters {
		if i > 0 {
			b.WriteString(",")
		}
		base := i * 6
		b.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6))
		args = append(args, ch.ID, ch.MediaItemID, ch.Title, ch.StartTimeMs, ch.EndTimeMs, ch.SortOrder)
	}

	if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ChapterRepository) GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*library.Chapter, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, media_item_id, title, start_time_ms, end_time_ms, sort_order
		 FROM chapters WHERE media_item_id = $1 ORDER BY sort_order`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Chapter
	for rows.Next() {
		ch := &library.Chapter{}
		if err := rows.Scan(&ch.ID, &ch.MediaItemID, &ch.Title, &ch.StartTimeMs, &ch.EndTimeMs, &ch.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, ch)
	}
	return result, rows.Err()
}
