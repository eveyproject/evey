package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/gravity-zero/evey/internal/domain/plugin"
)

type PluginRepository struct {
	db *sql.DB
}

func NewPluginRepository(db *sql.DB) *PluginRepository {
	return &PluginRepository{db: db}
}

func (r *PluginRepository) Create(ctx context.Context, p *plugin.Plugin) error {
	capJSON, err := json.Marshal(p.Capabilities)
	if err != nil {
		return err
	}
	permJSON, err := json.Marshal(p.Permissions)
	if err != nil {
		return err
	}
	schemaJSON, err := json.Marshal(p.ConfigSchema)
	if err != nil {
		return err
	}
	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO plugins (id, name, version, description, author, language, capabilities, permissions,
		 config_schema, config, enabled, status, error_message, installed_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		p.ID, p.Name, p.Version, p.Description, p.Author, p.Language,
		capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.InstalledAt, p.UpdatedAt,
	)
	return err
}

func (r *PluginRepository) GetByID(ctx context.Context, id string) (*plugin.Plugin, error) {
	return r.scanPlugin(r.db.QueryRowContext(ctx,
		`SELECT id, name, version, description, author, language, capabilities, permissions,
		 config_schema, config, enabled, status, error_message, installed_at, updated_at
		 FROM plugins WHERE id = $1`, id))
}

func (r *PluginRepository) GetByName(ctx context.Context, name string) (*plugin.Plugin, error) {
	return r.scanPlugin(r.db.QueryRowContext(ctx,
		`SELECT id, name, version, description, author, language, capabilities, permissions,
		 config_schema, config, enabled, status, error_message, installed_at, updated_at
		 FROM plugins WHERE name = $1`, name))
}

func (r *PluginRepository) List(ctx context.Context) ([]*plugin.Plugin, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, version, description, author, language, capabilities, permissions,
		 config_schema, config, enabled, status, error_message, installed_at, updated_at
		 FROM plugins ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*plugin.Plugin
	for rows.Next() {
		p := &plugin.Plugin{}
		var capJSON, permJSON, schemaJSON, configJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
			&capJSON, &permJSON, &schemaJSON, &configJSON,
			&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(capJSON, &p.Capabilities); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "capabilities"), slog.Any("err", err))
		}
		if err := json.Unmarshal(permJSON, &p.Permissions); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
		}
		if err := json.Unmarshal(schemaJSON, &p.ConfigSchema); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "config_schema"), slog.Any("err", err))
		}
		if err := json.Unmarshal(configJSON, &p.Config); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "config"), slog.Any("err", err))
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *PluginRepository) Update(ctx context.Context, p *plugin.Plugin) error {
	capJSON, err := json.Marshal(p.Capabilities)
	if err != nil {
		return err
	}
	permJSON, err := json.Marshal(p.Permissions)
	if err != nil {
		return err
	}
	schemaJSON, err := json.Marshal(p.ConfigSchema)
	if err != nil {
		return err
	}
	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE plugins SET name=$1, version=$2, description=$3, author=$4, language=$5,
		 capabilities=$6, permissions=$7, config_schema=$8, config=$9,
		 enabled=$10, status=$11, error_message=$12, updated_at=$13 WHERE id=$14`,
		p.Name, p.Version, p.Description, p.Author, p.Language,
		capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.UpdatedAt, p.ID,
	)
	return err
}

func (r *PluginRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = $1`, id)
	return err
}

func (r *PluginRepository) scanPlugin(row *sql.Row) (*plugin.Plugin, error) {
	p := &plugin.Plugin{}
	var capJSON, permJSON, schemaJSON, configJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
		&capJSON, &permJSON, &schemaJSON, &configJSON,
		&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(capJSON, &p.Capabilities); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "capabilities"), slog.Any("err", err))
	}
	if err := json.Unmarshal(permJSON, &p.Permissions); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "permissions"), slog.Any("err", err))
	}
	if err := json.Unmarshal(schemaJSON, &p.ConfigSchema); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "config_schema"), slog.Any("err", err))
	}
	if err := json.Unmarshal(configJSON, &p.Config); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "config"), slog.Any("err", err))
	}
	return p, nil
}
