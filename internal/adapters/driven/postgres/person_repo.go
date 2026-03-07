package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gravity-zero/evey/internal/domain/library"
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
		`INSERT INTO people (id, name, provider_ids, thumb_path)
		 VALUES ($1,$2,$3,$4)`,
		person.ID, person.Name, providerJSON, person.ThumbPath,
	)
	return err
}

func (r *PersonRepository) GetByID(ctx context.Context, id string) (*library.Person, error) {
	return r.scanPerson(r.db.QueryRowContext(ctx,
		`SELECT id, name, provider_ids, thumb_path
		 FROM people WHERE id = $1`, id))
}

func (r *PersonRepository) GetByName(ctx context.Context, name string) (*library.Person, error) {
	return r.scanPerson(r.db.QueryRowContext(ctx,
		`SELECT id, name, provider_ids, thumb_path
		 FROM people WHERE name = $1`, name))
}

func (r *PersonRepository) List(ctx context.Context) ([]*library.Person, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, provider_ids, thumb_path
		 FROM people ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Person
	for rows.Next() {
		person := &library.Person{}
		var providerJSON []byte
		if err := rows.Scan(&person.ID, &person.Name, &providerJSON, &person.ThumbPath); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(providerJSON, &person.ProviderIDs)
		result = append(result, person)
	}
	return result, rows.Err()
}

func (r *PersonRepository) scanPerson(row *sql.Row) (*library.Person, error) {
	person := &library.Person{}
	var providerJSON []byte
	err := row.Scan(&person.ID, &person.Name, &providerJSON, &person.ThumbPath)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(providerJSON, &person.ProviderIDs)
	return person, nil
}
