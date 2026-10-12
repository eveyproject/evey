package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type NotificationSettingsRepository struct {
	db *sql.DB
}

func NewNotificationSettingsRepository(db *sql.DB) *NotificationSettingsRepository {
	return &NotificationSettingsRepository{db: db}
}

func (r *NotificationSettingsRepository) GetByProfile(ctx context.Context, profileID shared.ProfileID) (*notification.Settings, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification_settings.GetByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	s := &notification.Settings{ProfileID: profileID}
	var ml, sl, msl string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT movie_level, series_level, music_level FROM notification_settings WHERE profile_id = $1`, string(profileID),
	).Scan(&ml, &sl, &msl)
	if err == sql.ErrNoRows {
		// fall back to global defaults
		return r.GetGlobalDefaults(ctx)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	s.MovieLevel = notification.Level(ml)
	s.SeriesLevel = notification.Level(sl)
	s.MusicLevel = notification.Level(msl)
	s.ProfileID = profileID
	return s, nil
}

func (r *NotificationSettingsRepository) GetByProfiles(ctx context.Context, profileIDs []shared.ProfileID) (map[shared.ProfileID]*notification.Settings, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification_settings.GetByProfiles")
	defer span.End()
	span.SetAttributes(attribute.Int("profile.count", len(profileIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	result := make(map[shared.ProfileID]*notification.Settings, len(profileIDs))
	if len(profileIDs) == 0 {
		return result, nil
	}
	strProfileIDs := make([]string, len(profileIDs))
	for i, id := range profileIDs {
		strProfileIDs[i] = string(id)
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT profile_id, movie_level, series_level, music_level FROM notification_settings WHERE profile_id = ANY($1)`,
		pq.Array(strProfileIDs),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid, ml, sl, msl string
		if scanErr := rows.Scan(&pid, &ml, &sl, &msl); scanErr != nil {
			span.RecordError(scanErr)
			span.SetStatus(codes.Error, scanErr.Error())
			return nil, scanErr
		}
		result[shared.ProfileID(pid)] = &notification.Settings{
			ProfileID:   shared.ProfileID(pid),
			MovieLevel:  notification.Level(ml),
			SeriesLevel: notification.Level(sl),
			MusicLevel:  notification.Level(msl),
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		span.RecordError(rowsErr)
		span.SetStatus(codes.Error, rowsErr.Error())
		return nil, rowsErr
	}
	// fill missing profiles with global defaults
	defaults, err := r.GetGlobalDefaults(ctx)
	if err != nil {
		defaults = &notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}
	}
	for _, pid := range profileIDs {
		if _, ok := result[pid]; !ok {
			cp := *defaults
			cp.ProfileID = pid
			result[pid] = &cp
		}
	}
	return result, nil
}

func (r *NotificationSettingsRepository) Upsert(ctx context.Context, s *notification.Settings) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification_settings.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(s.ProfileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO notification_settings (profile_id, movie_level, series_level, music_level)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (profile_id) DO UPDATE SET
		   movie_level = EXCLUDED.movie_level,
		   series_level = EXCLUDED.series_level,
		   music_level = EXCLUDED.music_level`,
		string(s.ProfileID), string(s.MovieLevel), string(s.SeriesLevel), string(s.MusicLevel),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationSettingsRepository) GetGlobalDefaults(ctx context.Context) (*notification.Settings, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification_settings.GetGlobalDefaults")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	s := &notification.Settings{}
	var ml, sl, msl string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT movie_level, series_level, music_level FROM notification_settings_global WHERE id = 1`,
	).Scan(&ml, &sl, &msl)
	if err != nil {
		return &notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}, nil //nolint:nilerr
	}
	s.MovieLevel = notification.Level(ml)
	s.SeriesLevel = notification.Level(sl)
	s.MusicLevel = notification.Level(msl)
	return s, nil
}

func (r *NotificationSettingsRepository) UpsertGlobalDefaults(ctx context.Context, s *notification.Settings) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification_settings.UpsertGlobalDefaults")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE notification_settings_global SET movie_level=$1, series_level=$2, music_level=$3 WHERE id=1`,
		string(s.MovieLevel), string(s.SeriesLevel), string(s.MusicLevel),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ notification.SettingsRepository = (*NotificationSettingsRepository)(nil)
