package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/user"
)

type RoleRepository struct {
	db *sql.DB
}

func NewRoleRepository(db *sql.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) Create(ctx context.Context, role *user.Role) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	permJSON, err := json.Marshal(role.Permissions)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO roles (id, name, description, permissions, is_system)
		 VALUES ($1,$2,$3,$4,$5)`,
		role.ID, role.Name, role.Description, permJSON, role.IsSystem,
	)
	return err
}

func (r *RoleRepository) GetByID(ctx context.Context, id string) (*user.Role, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanRole(r.db.QueryRowContext(ctx,
		`SELECT id, name, description, permissions, is_system
		 FROM roles WHERE id = $1`, id))
}

func (r *RoleRepository) GetByName(ctx context.Context, name string) (*user.Role, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanRole(r.db.QueryRowContext(ctx,
		`SELECT id, name, description, permissions, is_system
		 FROM roles WHERE name = $1`, name))
}

func (r *RoleRepository) List(ctx context.Context) ([]*user.Role, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, permissions, is_system
		 FROM roles ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*user.Role
	for rows.Next() {
		role := &user.Role{}
		var permJSON []byte
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &permJSON, &role.IsSystem); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(permJSON, &role.Permissions); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
		}
		result = append(result, role)
	}
	return result, rows.Err()
}

func (r *RoleRepository) scanRole(row *sql.Row) (*user.Role, error) {
	role := &user.Role{}
	var permJSON []byte
	err := row.Scan(&role.ID, &role.Name, &role.Description, &permJSON, &role.IsSystem)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(permJSON, &role.Permissions); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
	}
	return role, nil
}

var _ user.RoleRepository = (*RoleRepository)(nil)
