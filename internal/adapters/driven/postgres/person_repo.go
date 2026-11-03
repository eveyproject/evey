package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	person.Name = sanitizePersonName(person.Name)
	if tmdb := person.ProviderIDs["tmdb"]; tmdb != "" {
		err = r.createByProviderID(ctx, person, tmdb, providerJSON)
	} else {
		err = r.createByName(ctx, person, providerJSON)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// createByName inserts a person the provider gave no id for: its name is its
// identity, unique among the id-less rows only. RETURNING id so person.ID
// reflects the PERSISTED row. A concurrent enrichment can create the same
// person between the caller's GetByName miss and this insert; a plain DO
// NOTHING would silently keep the existing row but leave person.ID as the
// caller's fresh uuid, so the downstream media_people cast link would target
// a non-existent person id and the FK would drop the link. DO UPDATE SET
// name=name is a no-op that preserves every existing field while letting
// RETURNING hand back the surviving row id.
func (r *PersonRepository) createByName(ctx context.Context, person *library.Person, providerJSON []byte) error {
	return txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`INSERT INTO people (id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (name) WHERE COALESCE(provider_ids->>'tmdb', '') = '' DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		person.ID, person.Name, providerJSON, person.ThumbPath, person.Biography, person.Birthday, person.Deathday, person.PlaceOfBirth, person.KnownFor, person.BioCheckedAt,
	).Scan(&person.ID)
}

// createByProviderID inserts a person the provider knows under tmdb, unless
// a row already carries that id: then person.ID becomes that row's id, for
// the same cast-link reason as createByName. The id index cannot be unique
// (rows credited under two spellings of one id predate it), so the race
// between two enrichments of one person is closed by a lock on the id held
// to the end of the transaction instead of by a conflict target.
func (r *PersonRepository) createByProviderID(ctx context.Context, person *library.Person, tmdb string, providerJSON []byte) error {
	return NewTxUnit(r.db).Do(ctx, func(ctx context.Context) error {
		ex := txFromCtx(ctx, r.db)
		existing, err := lockProviderID(ctx, ex, "tmdb", tmdb)
		if err != nil {
			return err
		}
		if existing != "" {
			person.ID = existing
			return nil
		}
		_, err = ex.ExecContext(ctx,
			`INSERT INTO people (id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			person.ID, person.Name, providerJSON, person.ThumbPath, person.Biography, person.Birthday, person.Deathday, person.PlaceOfBirth, person.KnownFor, person.BioCheckedAt,
		)
		return err
	})
}

// lockProviderID takes the transaction lock that serialises every write
// giving a person this provider id, then returns the row already holding it
// ("" when none). Create and ClaimProviderID both go through it.
func lockProviderID(ctx context.Context, ex dbExecutor, provider, id string) (string, error) {
	if _, err := ex.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "people."+provider+":"+id); err != nil {
		return "", err
	}
	query := `SELECT id FROM people WHERE provider_ids->>'tmdb' = $1 ORDER BY id LIMIT 1`
	args := []any{id}
	if provider != "tmdb" {
		query = `SELECT id FROM people WHERE provider_ids->>$2 = $1 ORDER BY id LIMIT 1`
		args = append(args, provider)
	}
	var existing string
	err := ex.QueryRowContext(ctx, query, args...).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return existing, err
}

func (r *PersonRepository) ClaimProviderID(ctx context.Context, personID, provider, id string) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.ClaimProviderID")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", personID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	owner := personID
	err := NewTxUnit(r.db).Do(ctx, func(ctx context.Context) error {
		ex := txFromCtx(ctx, r.db)
		existing, err := lockProviderID(ctx, ex, provider, id)
		if err != nil {
			return err
		}
		if existing != "" {
			owner = existing
			return nil
		}
		_, err = ex.ExecContext(ctx,
			// A person created without ids stores JSON null, and null || {...}
			// is an array that no ->> lookup finds: start from an object.
			`UPDATE people SET provider_ids = COALESCE(NULLIF(provider_ids, 'null'::jsonb), '{}'::jsonb) || jsonb_build_object($2::text, $3::text) WHERE id = $1`,
			personID, provider, id)
		return err
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	return owner, nil
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
		 FROM people WHERE name = $1
		 ORDER BY (COALESCE(provider_ids->>'tmdb', '') <> ''), id LIMIT 1`, name))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return person, nil
}

func (r *PersonRepository) GetByProviderID(ctx context.Context, provider, id string) (*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.person.GetByProviderID")
	defer span.End()
	span.SetAttributes(attribute.String("person.provider", provider))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// The tmdb key is spelled out so the expression index serves it; another
	// provider key goes through the generic, unindexed form. Rows sharing one
	// id may exist (the index is not unique): the lowest id is the stable pick.
	query := `SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at
		 FROM people WHERE provider_ids->>'tmdb' = $1 ORDER BY id LIMIT 1`
	args := []any{id}
	if provider != "tmdb" {
		query = `SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for, bio_checked_at
		 FROM people WHERE provider_ids->>$2 = $1 ORDER BY id LIMIT 1`
		args = append(args, provider)
	}
	person, err := r.scanPerson(txFromCtx(ctx, r.db).QueryRowContext(ctx, query, args...))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return person, nil
}

// GetByNamesBatch returns a name→Person map for the given names. Missing
// names are simply absent from the map. Single query with ANY($1) so a
// cast of 50 names doesn't translate to 50 round-trips. Among homonyms the
// row without a provider id is the one returned, as in GetByName.
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
		 FROM (SELECT DISTINCT ON (name) * FROM people WHERE name = ANY($1)
		       ORDER BY name, (COALESCE(provider_ids->>'tmdb', '') <> ''), id) p`, pq.Array(names))
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

// sanitizePersonName drops what Postgres refuses in a TEXT value (invalid
// UTF-8, U+0000): a provider name carrying either failed the insert and the
// whole cast with it, on every enrichment.
func sanitizePersonName(name string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(name, ""), "\x00", "")
}
