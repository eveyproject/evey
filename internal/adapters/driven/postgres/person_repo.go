package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type PersonRepository struct {
	db *sql.DB
}

func NewPersonRepository(db *sql.DB) *PersonRepository {
	return &PersonRepository{db: db}
}

func (r *PersonRepository) Create(ctx context.Context, person *library.Person) error {
	providerJSON, err := json.Marshal(person.ProviderIDs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO people (id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (name) DO NOTHING`,
		person.ID, person.Name, providerJSON, person.ThumbPath, person.Biography, person.Birthday, person.Deathday, person.PlaceOfBirth, person.KnownFor,
	)
	return err
}

func (r *PersonRepository) GetByID(ctx context.Context, id string) (*library.Person, error) {
	return r.scanPerson(r.db.QueryRowContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for
		 FROM people WHERE id = $1`, id))
}

func (r *PersonRepository) GetByName(ctx context.Context, name string) (*library.Person, error) {
	return r.scanPerson(r.db.QueryRowContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for
		 FROM people WHERE name = $1`, name))
}

func (r *PersonRepository) List(ctx context.Context) ([]*library.Person, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, provider_ids, thumb_path, biography, birthday, deathday, place_of_birth, known_for
		 FROM people ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Person
	for rows.Next() {
		person := &library.Person{}
		var providerJSON []byte
		if err := rows.Scan(&person.ID, &person.Name, &providerJSON, &person.ThumbPath, &person.Biography, &person.Birthday, &person.Deathday, &person.PlaceOfBirth, &person.KnownFor); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(providerJSON, &person.ProviderIDs); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "provider_ids"), slog.Any("err", err))
		}
		result = append(result, person)
	}
	return result, rows.Err()
}

func (r *PersonRepository) Update(ctx context.Context, person *library.Person) error {
	providerJSON, err := json.Marshal(person.ProviderIDs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE people SET thumb_path=$2, biography=$3, birthday=$4, deathday=$5, place_of_birth=$6, known_for=$7, provider_ids=$8
		 WHERE id=$1`,
		person.ID, person.ThumbPath, person.Biography, person.Birthday, person.Deathday, person.PlaceOfBirth, person.KnownFor, providerJSON,
	)
	return err
}

func (r *PersonRepository) scanPerson(row *sql.Row) (*library.Person, error) {
	person := &library.Person{}
	var providerJSON []byte
	err := row.Scan(&person.ID, &person.Name, &providerJSON, &person.ThumbPath, &person.Biography, &person.Birthday, &person.Deathday, &person.PlaceOfBirth, &person.KnownFor)
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
