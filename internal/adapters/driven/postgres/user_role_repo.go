package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type UserRoleRepository struct {
	db *sql.DB
}

func NewUserRoleRepository(db *sql.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

func (r *UserRoleRepository) AssignRole(ctx context.Context, userID, roleID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2)
		 ON CONFLICT (user_id, role_id) DO NOTHING`,
		userID, roleID,
	)
	return err
}

func (r *UserRoleRepository) RemoveRole(ctx context.Context, userID, roleID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, userID, roleID)
	return err
}

func (r *UserRoleRepository) GetRolesByUserID(ctx context.Context, userID string) ([]*user.Role, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.id, r.name, r.description, r.permissions, r.is_system
		 FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1
		 ORDER BY r.name`, userID)
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

var _ user.UserRoleRepository = (*UserRoleRepository)(nil)
