package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const pluginColumns = `id, name, version, description, author, language, entrypoint, install_dir,
	 capabilities, permissions, config_schema, config, enabled, status, error_message, installed_at, updated_at, source_hash, analysis_report, signature`

type PluginRepository struct {
	db *sql.DB
}

func NewPluginRepository(db *sql.DB) *PluginRepository {
	return &PluginRepository{db: db}
}

func (r *PluginRepository) Create(ctx context.Context, p *plugin.Plugin) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.Create")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", p.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	capJSON, err := json.Marshal(p.Capabilities)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	permJSON, err := json.Marshal(p.Permissions)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	schemaJSON, err := json.Marshal(p.ConfigSchema)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	var reportJSON []byte
	if p.AnalysisReport != nil {
		reportJSON, err = json.Marshal(p.AnalysisReport)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	var sigJSON []byte
	if p.Signature != nil {
		sigJSON, err = json.Marshal(p.Signature)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO plugins (`+pluginColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		p.ID, p.Name, p.Version, p.Description, p.Author, p.Language, p.Entrypoint, p.InstallDir,
		capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.InstalledAt, p.UpdatedAt, p.SourceHash, reportJSON, sigJSON,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (r *PluginRepository) GetByID(ctx context.Context, id string) (*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	p, err := r.scanPlugin(r.db.QueryRowContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins WHERE id = $1`, id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return p, err
}

func (r *PluginRepository) GetByName(ctx context.Context, name string) (*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.GetByName")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	p, err := r.scanPlugin(r.db.QueryRowContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins WHERE name = $1`, name))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return p, err
}

func (r *PluginRepository) List(ctx context.Context) ([]*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins ORDER BY name LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*plugin.Plugin
	for rows.Next() {
		p := &plugin.Plugin{}
		var capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
			&p.Entrypoint, &p.InstallDir, &capJSON, &permJSON, &schemaJSON, &configJSON,
			&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt, &p.SourceHash, &reportJSON, &sigJSON); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		unmarshalPluginJSON(p, capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON)
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *PluginRepository) Update(ctx context.Context, p *plugin.Plugin) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.Update")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", p.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	capJSON, err := json.Marshal(p.Capabilities)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	permJSON, err := json.Marshal(p.Permissions)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	schemaJSON, err := json.Marshal(p.ConfigSchema)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	var reportJSON []byte
	if p.AnalysisReport != nil {
		reportJSON, err = json.Marshal(p.AnalysisReport)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	var sigJSON []byte
	if p.Signature != nil {
		sigJSON, err = json.Marshal(p.Signature)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE plugins SET name=$1, version=$2, description=$3, author=$4, language=$5,
		 entrypoint=$6, install_dir=$7, capabilities=$8, permissions=$9, config_schema=$10, config=$11,
		 enabled=$12, status=$13, error_message=$14, updated_at=$15, source_hash=$16, analysis_report=$17,
		 signature=$18 WHERE id=$19`,
		p.Name, p.Version, p.Description, p.Author, p.Language,
		p.Entrypoint, p.InstallDir, capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.UpdatedAt, p.SourceHash, reportJSON, sigJSON, p.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *PluginRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM plugins WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *PluginRepository) scanPlugin(row *sql.Row) (*plugin.Plugin, error) {
	p := &plugin.Plugin{}
	var capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON []byte
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
		&p.Entrypoint, &p.InstallDir, &capJSON, &permJSON, &schemaJSON, &configJSON,
		&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt, &p.SourceHash, &reportJSON, &sigJSON)
	if err != nil {
		return nil, err
	}
	unmarshalPluginJSON(p, capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON)
	return p, nil
}

func unmarshalPluginJSON(p *plugin.Plugin, capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON []byte) {
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
	if len(sigJSON) > 0 {
		var sig plugin.SignatureStatus
		if err := json.Unmarshal(sigJSON, &sig); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "signature"), slog.Any("err", err))
		} else {
			p.Signature = &sig
		}
	}
}

var _ plugin.PluginRepository = (*PluginRepository)(nil)
