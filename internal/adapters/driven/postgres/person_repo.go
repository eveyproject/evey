package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type PersonRepository struct {
	db *sql.DB
}

func NewPersonRepository(db *sql.DB) *PersonRepository {
	return &PersonRepository{db: db}
}

func (r *PersonRepository) Create(ctx context.Context, person *library.Person) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.Create")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", person.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	providerJSON, err := json.Marshal(person.ProviderIDs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	// RETURNING id so person.ID reflects the PERSISTED row. A concurrent
	// enrichment can create the same person (unique name) between the caller's
	// GetByName miss and this insert; a plain DO NOTHING would silently keep the
	// existing row but leave person.ID as the caller's fresh uuid, so the
	// downstream media_people cast link would target a non-existent person id and
	// the FK would drop the link. DO UPDATE SET name=name is a no-op that
	// preserves every existing field while letting RETURNING hand back the
	// surviving row id.
	if err = txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`INSERT INTO people (id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		person.ID, person.Name, providerJSON, person.ThumbPath, person.Biography, person.Birthday, person.Deathday, person.PlaceOfBirth, person.KnownFor, person.BioCheckedAt,
	).Scan(&person.ID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PersonRepository) GetByID(ctx context.Context, id string) (*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	person, err := r.scanPerson(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at
		 FROM people WHERE id = $1`, id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return person, nil
}

func (r *PersonRepository) GetByName(ctx context.Context, name string) (*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.GetByName")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	person, err := r.scanPerson(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at
		 FROM people WHERE name = $1`, name))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return person, nil
}

// GetByNamesBatch returns a name→Person map for the given names. Missing
// names are simply absent from the map. Single query with ANY($1) so a
// cast of 50 names doesn't translate to 50 round-trips.
func (r *PersonRepository) GetByNamesBatch(ctx context.Context, names []string) (map[string]*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.GetByNamesBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("person.count", len(names)))
	out := make(map[string]*library.Person, len(names))
	if len(names) == 0 {
		return out, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at
		 FROM people WHERE name = ANY($1)`, pq.Array(names))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		p := &library.Person{}
		var providerJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &providerJSON, &p.ThumbPath, &p.Biography, &p.Birthday, &p.Deathday, &p.PlaceOfBirth, &p.KnownFor, &p.BioCheckedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(providerJSON, &p.ProviderIDs); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "provider_ids"), slog.Any("err", err))
		}
		out[p.Name] = p
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

func (r *PersonRepository) List(ctx context.Context) ([]*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at
		 FROM people ORDER BY name LIMIT 10000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.Person
	for rows.Next() {
		person := &library.Person{}
		var providerJSON []byte
		if err := rows.Scan(&person.ID, &person.Name, &providerJSON, &person.ThumbPath, &person.Biography, &person.Birthday, &person.Deathday, &person.PlaceOfBirth, &person.KnownFor, &person.BioCheckedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(providerJSON, &person.ProviderIDs); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "provider_ids"), slog.Any("err", err))
		}
		result = append(result, person)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *PersonRepository) Update(ctx context.Context, person *library.Person) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.Update")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", person.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	providerJSON, err := json.Marshal(person.ProviderIDs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE people SET thumb_path=$2, biography=$3, birthday=$4, deathday=$5, place_of_birth=$6, known_for=$7, provider_ids=$8, bio_checked_at=$9
		 WHERE id=$1`,
		person.ID, person.ThumbPath, person.Biography, person.Birthday, person.Deathday, person.PlaceOfBirth, person.KnownFor, providerJSON, person.BioCheckedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PersonRepository) scanPerson(row *sql.Row) (*library.Person, error) {
	person := &library.Person{}
	var providerJSON []byte
	err := row.Scan(&person.ID, &person.Name, &providerJSON, &person.ThumbPath, &person.Biography, &person.Birthday, &person.Deathday, &person.PlaceOfBirth, &person.KnownFor, &person.BioCheckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(providerJSON, &person.ProviderIDs); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "provider_ids"), slog.Any("err", err))
	}
	return person, nil
}

var _ library.PersonRepository = (*PersonRepository)(nil)
