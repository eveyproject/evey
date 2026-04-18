package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
)

const pluginColumns = `id, name, version, description, author, language, entrypoint, install_dir,
	 capabilities, permissions, config_schema, config, enabled, status, error_message, installed_at, updated_at, source_hash, analysis_report`

type PluginRepository struct {
	db *sql.DB
}

func NewPluginRepository(db *sql.DB) *PluginRepository {
	return &PluginRepository{db: db}
}

func (r *PluginRepository) Create(ctx context.Context, p *plugin.Plugin) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
	var reportJSON []byte
	if p.AnalysisReport != nil {
		reportJSON, err = json.Marshal(p.AnalysisReport)
		if err != nil {
			return err
		}
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO plugins (`+pluginColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		p.ID, p.Name, p.Version, p.Description, p.Author, p.Language, p.Entrypoint, p.InstallDir,
		capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.InstalledAt, p.UpdatedAt, p.SourceHash, reportJSON,
	)
	return err
}

func (r *PluginRepository) GetByID(ctx context.Context, id string) (*plugin.Plugin, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanPlugin(r.db.QueryRowContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins WHERE id = $1`, id))
}

func (r *PluginRepository) GetByName(ctx context.Context, name string) (*plugin.Plugin, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanPlugin(r.db.QueryRowContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins WHERE name = $1`, name))
}

func (r *PluginRepository) List(ctx context.Context) ([]*plugin.Plugin, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*plugin.Plugin
	for rows.Next() {
		p := &plugin.Plugin{}
		var capJSON, permJSON, schemaJSON, configJSON, reportJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
			&p.Entrypoint, &p.InstallDir, &capJSON, &permJSON, &schemaJSON, &configJSON,
			&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt, &p.SourceHash, &reportJSON); err != nil {
			return nil, err
		}
		unmarshalPluginJSON(p, capJSON, permJSON, schemaJSON, configJSON, reportJSON)
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *PluginRepository) Update(ctx context.Context, p *plugin.Plugin) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
	var reportJSON []byte
	if p.AnalysisReport != nil {
		reportJSON, err = json.Marshal(p.AnalysisReport)
		if err != nil {
			return err
		}
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE plugins SET name=$1, version=$2, description=$3, author=$4, language=$5,
		 entrypoint=$6, install_dir=$7, capabilities=$8, permissions=$9, config_schema=$10, config=$11,
		 enabled=$12, status=$13, error_message=$14, updated_at=$15, source_hash=$16, analysis_report=$17 WHERE id=$18`,
		p.Name, p.Version, p.Description, p.Author, p.Language,
		p.Entrypoint, p.InstallDir, capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.UpdatedAt, p.SourceHash, reportJSON, p.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *PluginRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *PluginRepository) scanPlugin(row *sql.Row) (*plugin.Plugin, error) {
	p := &plugin.Plugin{}
	var capJSON, permJSON, schemaJSON, configJSON, reportJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
		&p.Entrypoint, &p.InstallDir, &capJSON, &permJSON, &schemaJSON, &configJSON,
		&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt, &p.SourceHash, &reportJSON)
	if err != nil {
		return nil, err
	}
	unmarshalPluginJSON(p, capJSON, permJSON, schemaJSON, configJSON, reportJSON)
	return p, nil
}

func unmarshalPluginJSON(p *plugin.Plugin, capJSON, permJSON, schemaJSON, configJSON, reportJSON []byte) {
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
	if len(reportJSON) > 0 {
		var rep plugin.AnalysisReport
		if err := json.Unmarshal(reportJSON, &rep); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "analysis_report"), slog.Any("err", err))
		} else {
			p.AnalysisReport = &rep
		}
	}
}

var _ plugin.PluginRepository = (*PluginRepository)(nil)
