package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/lib/pq"
)

type ProfileRepository struct {
	db *sql.DB
}

func NewProfileRepository(db *sql.DB) *ProfileRepository {
	return &ProfileRepository{db: db}
}

func (r *ProfileRepository) Create(ctx context.Context, p *user.Profile) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
	pMaxAge, pBlockedTags, pPINHash, pEnabled := parentalToColumns(p.Parental)
	locale := p.Locale
	if locale == "" {
		locale = "en"
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO profiles (id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		p.ID, p.UserID, p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault, p.IsKids,
		pq.Array(audioLangs), pq.Array(subLangs), p.MaxBitrateKbps,
		pMaxAge, pq.Array(pBlockedTags), pPINHash, pEnabled,
		locale, p.Region, p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	return nil
}

func (r *ProfileRepository) GetByID(ctx context.Context, id shared.ProfileID) (*user.Profile, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	p := &user.Profile{}
	var prefsJSON []byte
	var pMaxAge *int
	var pPINHash string
	var pBlockedTags []string
	var pEnabled bool
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at
		 FROM profiles WHERE id = $1`, string(id),
	).Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault, &p.IsKids,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.MaxBitrateKbps,
		&pMaxAge, pq.Array(&pBlockedTags), &pPINHash, &pEnabled,
		&p.Locale, &p.Region, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ProfileRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(prefsJSON, &p.Preferences); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "preferences"), slog.Any("err", err))
	}
	p.Parental = parentalFromColumns(pMaxAge, pBlockedTags, pPINHash, pEnabled)
	return p, nil
}

func (r *ProfileRepository) GetByUserID(ctx context.Context, userID shared.UserID) ([]*user.Profile, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at
		 FROM profiles WHERE user_id = $1 ORDER BY is_default DESC, name`, string(userID))
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
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
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
	pMaxAge, pBlockedTags, pPINHash, pEnabled := parentalToColumns(p.Parental)
	locale := p.Locale
	if locale == "" {
		locale = "en"
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE profiles SET name=$1, avatar_path=$2, preferences=$3, max_rating=$4, is_default=$5, is_kids=$6, audio_languages=$7, subtitle_languages=$8, max_bitrate_kbps=$9, parental_max_age=$10, parental_blocked_tags=$11, parental_pin_hash=$12, parental_enabled=$13, locale=$14, region=$15 WHERE id=$16`,
		p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault, p.IsKids,
		pq.Array(audioLangs), pq.Array(subLangs), p.MaxBitrateKbps,
		pMaxAge, pq.Array(pBlockedTags), pPINHash, pEnabled,
		locale, p.Region, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	return nil
}

func (r *ProfileRepository) GetAll(ctx context.Context) ([]*user.Profile, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at
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

func (r *ProfileRepository) Delete(ctx context.Context, id shared.ProfileID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	return nil
}

func scanProfile(rows *sql.Rows) (*user.Profile, error) {
	p := &user.Profile{}
	var prefsJSON []byte
	var pMaxAge *int
	var pPINHash string
	var pBlockedTags []string
	var pEnabled bool
	if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault, &p.IsKids,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.MaxBitrateKbps,
		&pMaxAge, pq.Array(&pBlockedTags), &pPINHash, &pEnabled,
		&p.Locale, &p.Region, &p.CreatedAt); err != nil {
		return nil, fmt.Errorf("scanProfile: %w", err)
	}
	if err := json.Unmarshal(prefsJSON, &p.Preferences); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "preferences"), slog.Any("err", err))
	}
	p.Parental = parentalFromColumns(pMaxAge, pBlockedTags, pPINHash, pEnabled)
	return p, nil
}

func parentalToColumns(ps *user.ParentalSettings) (*int, []string, string, bool) {
	if ps == nil {
		return nil, []string{}, "", false
	}
	tags := ps.BlockedTags
	if tags == nil {
		tags = []string{}
	}
	return ps.MaxAge, tags, ps.PINHash, ps.Enabled
}

func parentalFromColumns(maxAge *int, blockedTags []string, pinHash string, enabled bool) *user.ParentalSettings {
	if !enabled && maxAge == nil && pinHash == "" && len(blockedTags) == 0 {
		return nil
	}
	return &user.ParentalSettings{
		MaxAge:      maxAge,
		BlockedTags: blockedTags,
		PINHash:     pinHash,
		Enabled:     enabled,
	}
}

// ListAdminProfiles returns profiles whose user has is_admin=TRUE.
// Used by notification service to route security alerts.
func (r *ProfileRepository) ListAdminProfiles(ctx context.Context) ([]*user.Profile, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, p.user_id, p.name, p.avatar_path, p.preferences, p.max_rating, p.is_default, p.is_kids,
		 p.audio_languages, p.subtitle_languages, p.max_bitrate_kbps, p.parental_max_age, p.parental_blocked_tags,
		 p.parental_pin_hash, p.parental_enabled, p.locale, p.region, p.created_at
		 FROM profiles p
		 INNER JOIN users u ON u.id = p.user_id
		 WHERE u.is_admin = TRUE
		 ORDER BY p.user_id, p.is_default DESC, p.name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("ProfileRepo.ListAdminProfiles: %w", err)
	}
	defer rows.Close()
	var result []*user.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("ProfileRepo.ListAdminProfiles scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ProfileRepo.ListAdminProfiles rows: %w", err)
	}
	return result, nil
}

var _ user.ProfileRepository = (*ProfileRepository)(nil)
