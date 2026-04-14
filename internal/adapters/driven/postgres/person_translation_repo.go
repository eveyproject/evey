package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type PersonTranslationRepository struct {
	db *sql.DB
}

func NewPersonTranslationRepository(db *sql.DB) *PersonTranslationRepository {
	return &PersonTranslationRepository{db: db}
}

func (r *PersonTranslationRepository) Upsert(ctx context.Context, t *library.PersonTranslation) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO person_translations (person_id, locale, biography)
		VALUES ($1, $2, $3)
		ON CONFLICT (person_id, locale)
		DO UPDATE SET biography = EXCLUDED.biography
	`, t.PersonID, t.Locale, t.Biography)
	return err
}

func (r *PersonTranslationRepository) GetByPersonAndLocale(ctx context.Context, personID, locale string) (*library.PersonTranslation, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := r.db.QueryRowContext(ctx, `
		SELECT person_id, locale, biography
		FROM person_translations
		WHERE person_id = $1 AND locale = $2
	`, personID, locale)
	var t library.PersonTranslation
	if err := row.Scan(&t.PersonID, &t.Locale, &t.Biography); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *PersonTranslationRepository) GetByPerson(ctx context.Context, personID string) ([]*library.PersonTranslation, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT person_id, locale, biography
		FROM person_translations
		WHERE person_id = $1
		ORDER BY locale
	`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*library.PersonTranslation
	for rows.Next() {
		var t library.PersonTranslation
		if err := rows.Scan(&t.PersonID, &t.Locale, &t.Biography); err != nil {
			return nil, err
		}
		result = append(result, &t)
	}
	return result, rows.Err()
}

func (r *PersonTranslationRepository) DeleteByPerson(ctx context.Context, personID string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM person_translations WHERE person_id = $1`, personID)
	return err
}

var _ library.PersonTranslationRepository = (*PersonTranslationRepository)(nil)
