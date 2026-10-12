package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type RoleRepository struct {
	db *sql.DB
}

func NewRoleRepository(db *sql.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) Create(ctx context.Context, role *user.Role) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.role.Create")
	defer span.End()
	span.SetAttributes(attribute.String("role.id", role.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	permJSON, err := json.Marshal(role.Permissions)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO roles (id, name, description, permissions, is_system)
		 VALUES ($1,$2,$3,$4,$5)`,
		role.ID, role.Name, role.Description, permJSON, role.IsSystem,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *RoleRepository) GetByID(ctx context.Context, id string) (*user.Role, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.role.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("role.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	role, err := r.scanRole(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, description, permissions, is_system
		 FROM roles WHERE id = $1`, id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return role, nil
}

func (r *RoleRepository) GetByName(ctx context.Context, name string) (*user.Role, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.role.GetByName")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	role, err := r.scanRole(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, description, permissions, is_system
		 FROM roles WHERE name = $1`, name))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return role, nil
}

func (r *RoleRepository) List(ctx context.Context) ([]*user.Role, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.role.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, name, description, permissions, is_system
		 FROM roles ORDER BY name LIMIT 1000`)
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
