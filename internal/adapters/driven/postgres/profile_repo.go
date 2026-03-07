package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gravity-zero/evey/internal/domain/user"
	"github.com/lib/pq"
)

type ProfileRepository struct {
	db *sql.DB
}

func NewProfileRepository(db *sql.DB) *ProfileRepository {
	return &ProfileRepository{db: db}
}

func (r *ProfileRepository) Create(ctx context.Context, p *user.Profile) error {
	prefsJSON, err := json.Marshal(p.Preferences)
	if err != nil {
		return err
	}
	audioLangs := p.AudioLanguages
	if audioLangs == nil {
		audioLangs = []string{}
	}
	subLangs := p.SubtitleLanguages
	if subLangs == nil {
		subLangs = []string{}
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO profiles (id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		p.ID, p.UserID, p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault,
		pq.Array(audioLangs), pq.Array(subLangs), p.CreatedAt,
	)
	return err
}

func (r *ProfileRepository) GetByID(ctx context.Context, id string) (*user.Profile, error) {
	p := &user.Profile{}
	var prefsJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, created_at
		 FROM profiles WHERE id = $1`, id,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(prefsJSON, &p.Preferences)
	return p, nil
}

func (r *ProfileRepository) GetByUserID(ctx context.Context, userID string) ([]*user.Profile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, created_at
		 FROM profiles WHERE user_id = $1 ORDER BY is_default DESC, name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*user.Profile
	for rows.Next() {
		p := &user.Profile{}
		var prefsJSON []byte
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault,
			pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(prefsJSON, &p.Preferences)
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *ProfileRepository) Update(ctx context.Context, p *user.Profile) error {
	prefsJSON, err := json.Marshal(p.Preferences)
	if err != nil {
		return err
	}
	audioLangs := p.AudioLanguages
	if audioLangs == nil {
		audioLangs = []string{}
	}
	subLangs := p.SubtitleLanguages
	if subLangs == nil {
		subLangs = []string{}
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE profiles SET name=$1, avatar_path=$2, preferences=$3, max_rating=$4, is_default=$5, audio_languages=$6, subtitle_languages=$7 WHERE id=$8`,
		p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault,
		pq.Array(audioLangs), pq.Array(subLangs), p.ID,
	)
	return err
}

func (r *ProfileRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, id)
	return err
}
