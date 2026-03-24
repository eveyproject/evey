package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/gravity-zero/evey/internal/domain/notification"
)

type NotificationSettingsRepository struct {
	db *sql.DB
}

func NewNotificationSettingsRepository(db *sql.DB) *NotificationSettingsRepository {
	return &NotificationSettingsRepository{db: db}
}

func (r *NotificationSettingsRepository) GetByProfile(ctx context.Context, profileID string) (*notification.Settings, error) {
	s := &notification.Settings{ProfileID: profileID}
	var ml, sl, msl string
	err := r.db.QueryRowContext(ctx,
		`SELECT movie_level, series_level, music_level FROM notification_settings WHERE profile_id = $1`, profileID,
	).Scan(&ml, &sl, &msl)
	if err == sql.ErrNoRows {
		// fall back to global defaults
		return r.GetGlobalDefaults(ctx)
	}
	if err != nil {
		return nil, err
	}
	s.MovieLevel = notification.Level(ml)
	s.SeriesLevel = notification.Level(sl)
	s.MusicLevel = notification.Level(msl)
	s.ProfileID = profileID
	return s, nil
}

func (r *NotificationSettingsRepository) GetByProfiles(ctx context.Context, profileIDs []string) (map[string]*notification.Settings, error) {
	result := make(map[string]*notification.Settings, len(profileIDs))
	if len(profileIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT profile_id, movie_level, series_level, music_level FROM notification_settings WHERE profile_id = ANY($1)`,
		pq.Array(profileIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid, ml, sl, msl string
		if scanErr := rows.Scan(&pid, &ml, &sl, &msl); scanErr != nil {
			return nil, scanErr
		}
		result[pid] = &notification.Settings{
			ProfileID:   pid,
			MovieLevel:  notification.Level(ml),
			SeriesLevel: notification.Level(sl),
			MusicLevel:  notification.Level(msl),
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
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
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_settings (profile_id, movie_level, series_level, music_level)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (profile_id) DO UPDATE SET
		   movie_level = EXCLUDED.movie_level,
		   series_level = EXCLUDED.series_level,
		   music_level = EXCLUDED.music_level`,
		s.ProfileID, string(s.MovieLevel), string(s.SeriesLevel), string(s.MusicLevel),
	)
	return err
}

func (r *NotificationSettingsRepository) GetGlobalDefaults(ctx context.Context) (*notification.Settings, error) {
	s := &notification.Settings{}
	var ml, sl, msl string
	err := r.db.QueryRowContext(ctx,
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
	_, err := r.db.ExecContext(ctx,
		`UPDATE notification_settings_global SET movie_level=$1, series_level=$2, music_level=$3 WHERE id=1`,
		string(s.MovieLevel), string(s.SeriesLevel), string(s.MusicLevel),
	)
	return err
}

var _ notification.SettingsRepository = (*NotificationSettingsRepository)(nil)
