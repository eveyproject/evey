package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const pluginColumns = `id, name, version, description, author, language, entrypoint, install_dir,
	 capabilities, permissions, config_schema, config, enabled, status, error_message, installed_at, updated_at, source_hash, analysis_report, signature, kind, frontend, external_fetch, web_root`

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
	var frontendJSON []byte
	if len(p.Frontend) > 0 {
		frontendJSON, err = json.Marshal(p.Frontend)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	var extFetchJSON []byte
	if len(p.ExternalFetch) > 0 {
		extFetchJSON, err = json.Marshal(p.ExternalFetch)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	kind := p.Kind
	if kind == "" {
		kind = plugin.KindPlugin
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO plugins (`+pluginColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
		p.ID, p.Name, p.Version, p.Description, p.Author, p.Language, p.Entrypoint, p.InstallDir,
		capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.InstalledAt, p.UpdatedAt, p.SourceHash, reportJSON, sigJSON,
		string(kind), frontendJSON, extFetchJSON, p.WebRoot,
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
	p, err := r.scanPlugin(txFromCtx(ctx, r.db).QueryRowContext(ctx,
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
	p, err := r.scanPlugin(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT `+pluginColumns+` FROM plugins WHERE name = $1`, name))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return p, nil
}

func (r *PluginRepository) List(ctx context.Context) ([]*plugin.Plugin, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
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
		var capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON, frontendJSON, extFetchJSON []byte
		var kindStr string
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
			&p.Entrypoint, &p.InstallDir, &capJSON, &permJSON, &schemaJSON, &configJSON,
			&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt, &p.SourceHash, &reportJSON, &sigJSON,
			&kindStr, &frontendJSON, &extFetchJSON, &p.WebRoot); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		p.Kind = plugin.PluginKind(kindStr)
		if p.Kind == "" {
			if p.Language == "css" {
				p.Kind = plugin.KindTheme
			} else {
				p.Kind = plugin.KindPlugin
			}
		}
		unmarshalPluginJSON(p, capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON, frontendJSON, extFetchJSON)
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
	var frontendJSON []byte
	if len(p.Frontend) > 0 {
		frontendJSON, err = json.Marshal(p.Frontend)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	var extFetchJSON []byte
	if len(p.ExternalFetch) > 0 {
		extFetchJSON, err = json.Marshal(p.ExternalFetch)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	kind := p.Kind
	if kind == "" {
		kind = plugin.KindPlugin
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE plugins SET name=$1, version=$2, description=$3, author=$4, language=$5,
		 entrypoint=$6, install_dir=$7, capabilities=$8, permissions=$9, config_schema=$10, config=$11,
		 enabled=$12, status=$13, error_message=$14, updated_at=$15, source_hash=$16, analysis_report=$17,
		 signature=$18, kind=$19, frontend=$20, external_fetch=$21, web_root=$22 WHERE id=$23`,
		p.Name, p.Version, p.Description, p.Author, p.Language,
		p.Entrypoint, p.InstallDir, capJSON, permJSON, schemaJSON, configJSON,
		p.Enabled, p.Status, p.ErrorMessage, p.UpdatedAt, p.SourceHash, reportJSON, sigJSON,
		string(kind), frontendJSON, extFetchJSON, p.WebRoot, p.ID,
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

// UpdateConfig writes ONLY the config column, so a caller holding a stale
// snapshot cannot replay it over the columns it never touched. SetConfig - the
// one HostService RPC a plugin may call with NO permission - went through the
// blind 22-column Update, so a plugin looping on it restored a permission an
// admin had just revoked (external_fetch bit immediately: the broker re-reads
// it from the DB on every call).
func (r *PluginRepository) UpdateConfig(ctx context.Context, id string, config map[string]any) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.plugin.UpdateConfig")
	defer span.End()
	span.SetAttributes(attribute.String("plugin.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	configJSON, err := json.Marshal(config)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE plugins SET config=$1, updated_at=$2 WHERE id=$3`,
		configJSON, time.Now().UTC(), id,
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
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM plugins WHERE id = $1`, id)
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
	var capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON, frontendJSON, extFetchJSON []byte
	var kindStr string
	err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Language,
		&p.Entrypoint, &p.InstallDir, &capJSON, &permJSON, &schemaJSON, &configJSON,
		&p.Enabled, &p.Status, &p.ErrorMessage, &p.InstalledAt, &p.UpdatedAt, &p.SourceHash, &reportJSON, &sigJSON,
		&kindStr, &frontendJSON, &extFetchJSON, &p.WebRoot)
	if err != nil {
		return nil, err
	}
	p.Kind = plugin.PluginKind(kindStr)
	if p.Kind == "" {
		if p.Language == "css" {
			p.Kind = plugin.KindTheme
		} else {
			p.Kind = plugin.KindPlugin
		}
	}
	unmarshalPluginJSON(p, capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON, frontendJSON, extFetchJSON)
	return p, nil
}

func unmarshalPluginJSON(p *plugin.Plugin, capJSON, permJSON, schemaJSON, configJSON, reportJSON, sigJSON, frontendJSON, extFetchJSON []byte) {
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
	if len(frontendJSON) > 0 {
		if err := json.Unmarshal(frontendJSON, &p.Frontend); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "frontend"), slog.Any("err", err))
		}
	}
	if len(extFetchJSON) > 0 {
		if err := json.Unmarshal(extFetchJSON, &p.ExternalFetch); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "external_fetch"), slog.Any("err", err))
		}
	}
}

var _ plugin.PluginRepository = (*PluginRepository)(nil)
