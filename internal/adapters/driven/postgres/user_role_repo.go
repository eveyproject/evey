package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type UserRoleRepository struct {
	db *sql.DB
}

func NewUserRoleRepository(db *sql.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

func (r *UserRoleRepository) AssignRole(ctx context.Context, userID shared.UserID, roleID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user_role.AssignRole")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)), attribute.String("role.id", roleID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2)
		 ON CONFLICT (user_id, role_id) DO NOTHING`,
		string(userID), roleID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *UserRoleRepository) RemoveRole(ctx context.Context, userID shared.UserID, roleID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user_role.RemoveRole")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)), attribute.String("role.id", roleID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, string(userID), roleID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *UserRoleRepository) GetRolesByUserID(ctx context.Context, userID shared.UserID) ([]*user.Role, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user_role.GetRolesByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT r.id, r.name, r.description, r.permissions, r.is_system
		 FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1
		 ORDER BY r.name`, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*user.Role
	for rows.Next() {
		role := &user.Role{}
		var permJSON []byte
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &permJSON, &role.IsSystem); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(permJSON, &role.Permissions); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
		}
		result = append(result, role)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ user.UserRoleRepository = (*UserRoleRepository)(nil)
