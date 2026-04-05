package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

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
		return fmt.Errorf("ProfileRepo.Create marshal prefs: %w", err)
	}
	audioLangs := p.AudioLanguages
	if audioLangs == nil {
		audioLangs = []string{}
	}
	subLangs := p.SubtitleLanguages
	if subLangs == nil {
		subLangs = []string{}
	}
	pMaxRating, pBlockedTags, pPINHash, pEnabled := parentalToColumns(p.Parental)
	locale := p.Locale
	if locale == "" {
		locale = "en"
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO profiles (id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_rating, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		p.ID, p.UserID, p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault,
		pq.Array(audioLangs), pq.Array(subLangs), p.MaxBitrateKbps,
		pMaxRating, pq.Array(pBlockedTags), pPINHash, pEnabled,
		locale, p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	return nil
}

func (r *ProfileRepository) GetByID(ctx context.Context, id string) (*user.Profile, error) {
	p := &user.Profile{}
	var prefsJSON []byte
	var pMaxRating, pPINHash string
	var pBlockedTags []string
	var pEnabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_rating, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, created_at
		 FROM profiles WHERE id = $1`, id,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.MaxBitrateKbps,
		&pMaxRating, pq.Array(&pBlockedTags), &pPINHash, &pEnabled,
		&p.Locale, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ProfileRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(prefsJSON, &p.Preferences); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "preferences"), slog.Any("err", err))
	}
	p.Parental = parentalFromColumns(pMaxRating, pBlockedTags, pPINHash, pEnabled)
	return p, nil
}

func (r *ProfileRepository) GetByUserID(ctx context.Context, userID string) ([]*user.Profile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_rating, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, created_at
		 FROM profiles WHERE user_id = $1 ORDER BY is_default DESC, name`, userID)
	if err != nil {
		return nil, fmt.Errorf("ProfileRepo.GetByUserID: %w", err)
	}
	defer rows.Close()

	var result []*user.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("ProfileRepo.GetByUserID scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ProfileRepo.GetByUserID rows: %w", err)
	}
	return result, nil
}

func (r *ProfileRepository) Update(ctx context.Context, p *user.Profile) error {
	prefsJSON, err := json.Marshal(p.Preferences)
	if err != nil {
		return fmt.Errorf("ProfileRepo.Update marshal prefs: %w", err)
	}
	audioLangs := p.AudioLanguages
	if audioLangs == nil {
		audioLangs = []string{}
	}
	subLangs := p.SubtitleLanguages
	if subLangs == nil {
		subLangs = []string{}
	}
	pMaxRating, pBlockedTags, pPINHash, pEnabled := parentalToColumns(p.Parental)
	locale := p.Locale
	if locale == "" {
		locale = "en"
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE profiles SET name=$1, avatar_path=$2, preferences=$3, max_rating=$4, is_default=$5, audio_languages=$6, subtitle_languages=$7, max_bitrate_kbps=$8, parental_max_rating=$9, parental_blocked_tags=$10, parental_pin_hash=$11, parental_enabled=$12, locale=$13 WHERE id=$14`,
		p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault,
		pq.Array(audioLangs), pq.Array(subLangs), p.MaxBitrateKbps,
		pMaxRating, pq.Array(pBlockedTags), pPINHash, pEnabled,
		locale, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *ProfileRepository) GetAll(ctx context.Context) ([]*user.Profile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_rating, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, created_at
		 FROM profiles ORDER BY user_id, is_default DESC, name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("ProfileRepo.GetAll: %w", err)
	}
	defer rows.Close()

	var result []*user.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("ProfileRepo.GetAll scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ProfileRepo.GetAll rows: %w", err)
	}
	return result, nil
}

func (r *ProfileRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	return nil
}

func scanProfile(rows *sql.Rows) (*user.Profile, error) {
	p := &user.Profile{}
	var prefsJSON []byte
	var pMaxRating, pPINHash string
	var pBlockedTags []string
	var pEnabled bool
	if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.MaxBitrateKbps,
		&pMaxRating, pq.Array(&pBlockedTags), &pPINHash, &pEnabled,
		&p.Locale, &p.CreatedAt); err != nil {
		return nil, fmt.Errorf("scanProfile: %w", err)
	}
	if err := json.Unmarshal(prefsJSON, &p.Preferences); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "preferences"), slog.Any("err", err))
	}
	p.Parental = parentalFromColumns(pMaxRating, pBlockedTags, pPINHash, pEnabled)
	return p, nil
}

func parentalToColumns(ps *user.ParentalSettings) (string, []string, string, bool) {
	if ps == nil {
		return "", []string{}, "", false
	}
	tags := ps.BlockedTags
	if tags == nil {
		tags = []string{}
	}
	return ps.MaxRating, tags, ps.PINHash, ps.Enabled
}

func parentalFromColumns(maxRating string, blockedTags []string, pinHash string, enabled bool) *user.ParentalSettings {
	if !enabled && maxRating == "" && pinHash == "" && len(blockedTags) == 0 {
		return nil
	}
	return &user.ParentalSettings{
		MaxRating:   maxRating,
		BlockedTags: blockedTags,
		PINHash:     pinHash,
		Enabled:     enabled,
	}
}

var _ user.ProfileRepository = (*ProfileRepository)(nil)
