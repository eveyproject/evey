package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/mediarequest"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// mediaRequestAccessDefaultKey is the server_config key holding the global
// default (enabled/disabled) for users without an explicit override.
const mediaRequestAccessDefaultKey = "mediarequest.enabled_default"

// MediaRequestRepository persists media_requests (mediarequest.Repository)
// and the per-user access policy (mediarequest.Access) - the latter reads
// server_config for the global default and media_request_access for overrides.
type MediaRequestRepository struct {
	db *sql.DB
}

func NewMediaRequestRepository(db *sql.DB) *MediaRequestRepository {
	return &MediaRequestRepository{db: db}
}

func (r *MediaRequestRepository) Create(ctx context.Context, req *mediarequest.Request) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.Create")
	defer span.End()
	span.SetAttributes(attribute.String("media_request.id", req.ID), attribute.String("profile.id", string(req.ProfileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	answeredBy := sql.NullString{String: string(req.AnsweredBy), Valid: req.AnsweredBy != ""}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_requests (id, profile_id, user_id, media_type, provider, provider_id, title, year, poster_url, status, admin_note, answered_by, created_at, updated_at, answered_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		req.ID, req.ProfileID, req.UserID, string(req.MediaType), req.Provider, req.ProviderID,
		req.Title, req.Year, req.PosterURL, string(req.Status), req.AdminNote, answeredBy,
		req.CreatedAt, req.UpdatedAt, req.AnsweredAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaRequestRepository) GetByID(ctx context.Context, id string) (*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("media_request.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	req, err := scanMediaRequestRow(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, profile_id, user_id, media_type, provider, provider_id, title, year, poster_url, status, admin_note, answered_by, created_at, updated_at, answered_at
		 FROM media_requests WHERE id = $1`, id,
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mediarequest.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return req, nil
}

func (r *MediaRequestRepository) Update(ctx context.Context, req *mediarequest.Request) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.Update")
	defer span.End()
	span.SetAttributes(attribute.String("media_request.id", req.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	answeredBy := sql.NullString{String: string(req.AnsweredBy), Valid: req.AnsweredBy != ""}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE media_requests SET media_type=$1, provider=$2, provider_id=$3, title=$4, year=$5, poster_url=$6,
		 status=$7, admin_note=$8, answered_by=$9, updated_at=$10, answered_at=$11
		 WHERE id=$12`,
		string(req.MediaType), req.Provider, req.ProviderID, req.Title, req.Year, req.PosterURL,
		string(req.Status), req.AdminNote, answeredBy, req.UpdatedAt, req.AnsweredAt, req.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaRequestRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("media_request.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_requests WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaRequestRepository) ListByProfile(ctx context.Context, profileID string, limit, offset int) ([]*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.ListByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, profile_id, user_id, media_type, provider, provider_id, title, year, poster_url, status, admin_note, answered_by, created_at, updated_at, answered_at
		 FROM media_requests WHERE profile_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		profileID, limit, offset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanMediaRequestRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaRequestRepository) ListAll(ctx context.Context, statuses []mediarequest.Status, limit, offset int) ([]*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.ListAll")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var rows *sql.Rows
	var err error
	if len(statuses) == 0 {
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT id, profile_id, user_id, media_type, provider, provider_id, title, year, poster_url, status, admin_note, answered_by, created_at, updated_at, answered_at
			 FROM media_requests ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			limit, offset,
		)
	} else {
		statusStrs := make([]string, len(statuses))
		for i, s := range statuses {
			statusStrs[i] = string(s)
		}
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx,
			`SELECT id, profile_id, user_id, media_type, provider, provider_id, title, year, poster_url, status, admin_note, answered_by, created_at, updated_at, answered_at
			 FROM media_requests WHERE status = ANY($1) ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			pq.Array(statusStrs), limit, offset,
		)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanMediaRequestRows(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaRequestRepository) HasOpenRequest(ctx context.Context, profileID string, mediaType mediarequest.MediaType, providerID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request.HasOpenRequest")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var exists bool
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM media_requests WHERE profile_id = $1 AND media_type = $2 AND provider_id = $3 AND status <> 'rejected')`,
		profileID, string(mediaType), providerID,
	).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	return exists, nil
}

func scanMediaRequestRow(row *sql.Row) (*mediarequest.Request, error) {
	req := &mediarequest.Request{}
	var mediaType, status string
	var answeredBy sql.NullString
	if err := row.Scan(
		&req.ID, &req.ProfileID, &req.UserID, &mediaType, &req.Provider, &req.ProviderID,
		&req.Title, &req.Year, &req.PosterURL, &status, &req.AdminNote, &answeredBy,
		&req.CreatedAt, &req.UpdatedAt, &req.AnsweredAt,
	); err != nil {
		return nil, err
	}
	req.MediaType = mediarequest.MediaType(mediaType)
	req.Status = mediarequest.Status(status)
	if answeredBy.Valid {
		req.AnsweredBy = shared.ProfileID(answeredBy.String)
	}
	return req, nil
}

func scanMediaRequestRows(rows *sql.Rows) ([]*mediarequest.Request, error) {
	var result []*mediarequest.Request
	for rows.Next() {
		req := &mediarequest.Request{}
		var mediaType, status string
		var answeredBy sql.NullString
		if err := rows.Scan(
			&req.ID, &req.ProfileID, &req.UserID, &mediaType, &req.Provider, &req.ProviderID,
			&req.Title, &req.Year, &req.PosterURL, &status, &req.AdminNote, &answeredBy,
			&req.CreatedAt, &req.UpdatedAt, &req.AnsweredAt,
		); err != nil {
			return nil, err
		}
		req.MediaType = mediarequest.MediaType(mediaType)
		req.Status = mediarequest.Status(status)
		if answeredBy.Valid {
			req.AnsweredBy = shared.ProfileID(answeredBy.String)
		}
		result = append(result, req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

var _ mediarequest.Repository = (*MediaRequestRepository)(nil)

// MediaRequestAccessRepository implements mediarequest.Access: a global
// default stored in server_config, overridable per user in
// media_request_access.
type MediaRequestAccessRepository struct {
	db *sql.DB
}

func NewMediaRequestAccessRepository(db *sql.DB) *MediaRequestAccessRepository {
	return &MediaRequestAccessRepository{db: db}
}

func (r *MediaRequestAccessRepository) GlobalDefault(ctx context.Context) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request_access.GlobalDefault")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var value string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT value FROM server_config WHERE key = $1`, mediaRequestAccessDefaultKey,
	).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	return enabled, nil
}

func (r *MediaRequestAccessRepository) SetGlobalDefault(ctx context.Context, enabled bool) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request_access.SetGlobalDefault")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO server_config (key, value, updated_at) VALUES ($1, $2, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		mediaRequestAccessDefaultKey, strconv.FormatBool(enabled),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaRequestAccessRepository) Override(ctx context.Context, userID string) (bool, bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request_access.Override")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var enabled bool
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT enabled FROM media_request_access WHERE user_id = $1`, userID,
	).Scan(&enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, false, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, false, err
	}
	return enabled, true, nil
}

func (r *MediaRequestAccessRepository) SetOverride(ctx context.Context, userID string, enabled bool) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request_access.SetOverride")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_request_access (user_id, enabled, updated_at) VALUES ($1, $2, NOW())
		 ON CONFLICT (user_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()`,
		userID, enabled,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaRequestAccessRepository) ClearOverride(ctx context.Context, userID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request_access.ClearOverride")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_request_access WHERE user_id = $1`, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaRequestAccessRepository) ListOverrides(ctx context.Context) (map[string]bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media_request_access.ListOverrides")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `SELECT user_id, enabled FROM media_request_access`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var userID string
		var enabled bool
		if err := rows.Scan(&userID, &enabled); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[userID] = enabled
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ mediarequest.Access = (*MediaRequestAccessRepository)(nil)
