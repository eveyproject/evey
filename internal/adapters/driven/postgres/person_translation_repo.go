package postgres

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type PersonTranslationRepository struct {
	db *sql.DB
}

func NewPersonTranslationRepository(db *sql.DB) *PersonTranslationRepository {
	return &PersonTranslationRepository{db: db}
}

func (r *PersonTranslationRepository) Upsert(ctx context.Context, t *library.PersonTranslation) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.person_translation.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", t.PersonID), attribute.String("locale", t.Locale))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO person_translations (person_id, locale, biography)
		VALUES ($1, $2, $3)
		ON CONFLICT (person_id, locale)
		DO UPDATE SET biography = EXCLUDED.biography
	`, t.PersonID, t.Locale, t.Biography)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PersonTranslationRepository) GetByPersonAndLocale(ctx context.Context, personID, locale string) (*library.PersonTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person_translation.GetByPersonAndLocale")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", personID), attribute.String("locale", locale))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
		SELECT person_id, locale, biography
		FROM person_translations
		WHERE person_id = $1 AND locale = $2
	`, personID, locale)
	var t library.PersonTranslation
	if err := row.Scan(&t.PersonID, &t.Locale, &t.Biography); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return &t, nil
}

func (r *PersonTranslationRepository) GetByPerson(ctx context.Context, personID string) ([]*library.PersonTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person_translation.GetByPerson")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", personID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT person_id, locale, biography
		FROM person_translations
		WHERE person_id = $1
		ORDER BY locale
	`, personID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var result []*library.PersonTranslation
	for rows.Next() {
		var t library.PersonTranslation
		if err := rows.Scan(&t.PersonID, &t.Locale, &t.Biography); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, &t)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *PersonTranslationRepository) DeleteByPerson(ctx context.Context, personID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.person_translation.DeleteByPerson")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", personID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM person_translations WHERE person_id = $1`, personID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ library.PersonTranslationRepository = (*PersonTranslationRepository)(nil)
