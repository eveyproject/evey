package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type ProfileRepository struct {
	db *sql.DB
}

func NewProfileRepository(db *sql.DB) *ProfileRepository {
	return &ProfileRepository{db: db}
}

func (r *ProfileRepository) Create(ctx context.Context, p *user.Profile) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.Create")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(p.ID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	prefsJSON, err := json.Marshal(p.Preferences)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("create profile: %w", err)
	}
	return nil
}

// CreateWithinCap enforces the per-user profile cap and the insert atomically.
// It serialises concurrent creates for the same user with a tx-scoped advisory
// lock, counts the user's profiles under that lock, and inserts only when the
// count is below maxProfiles - closing the count-then-create TOCTOU that let
// concurrent requests overshoot the cap. maxProfiles <= 0 means unlimited.
func (r *ProfileRepository) CreateWithinCap(ctx context.Context, p *user.Profile, maxProfiles int) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.CreateWithinCap")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(p.ID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("ProfileRepo.CreateWithinCap begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				slog.Warn("ProfileRepo.CreateWithinCap rollback failed", slog.Any("err", rbErr))
			}
		}
	}()

	// Serialise concurrent creates for this user. The advisory lock is held
	// for the lifetime of the tx and released automatically on commit/rollback.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, string(p.UserID)); err != nil {
		span.RecordError(err)
		return fmt.Errorf("ProfileRepo.CreateWithinCap lock: %w", err)
	}
	if maxProfiles > 0 {
		var n int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM profiles WHERE user_id=$1`, string(p.UserID)).Scan(&n); err != nil {
			span.RecordError(err)
			return fmt.Errorf("ProfileRepo.CreateWithinCap count: %w", err)
		}
		if n >= maxProfiles {
			return user.ErrProfileCapReached
		}
	}
	// Reuse Create's INSERT, routed through this tx via ctxWithTx.
	if err = r.Create(ctxWithTx(ctx, tx), p); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("ProfileRepo.CreateWithinCap commit: %w", err)
	}
	committed = true
	return nil
}

func (r *ProfileRepository) GetByID(ctx context.Context, id shared.ProfileID) (*user.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	p := &user.Profile{}
	var prefsJSON []byte
	var pMaxAge *int
	var pPINHash string
	var pBlockedTags []string
	var pEnabled bool
	var scheduleJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at, last_active_at, access_schedule, schedule_override_until
		 FROM profiles WHERE id = $1`, string(id),
	).Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault, &p.IsKids,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.MaxBitrateKbps,
		&pMaxAge, pq.Array(&pBlockedTags), &pPINHash, &pEnabled,
		&p.Locale, &p.Region, &p.CreatedAt, &p.LastActiveAt, &scheduleJSON, &p.ScheduleOverrideUntil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// Map no-rows to the domain sentinel (the house convention used by the
		// other repo methods) so callers can distinguish a genuinely-missing
		// profile from a transient DB failure instead of collapsing both to 404.
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("ProfileRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(prefsJSON, &p.Preferences); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "preferences"), slog.Any("err", err))
	}
	p.Parental = parentalFromColumns(pMaxAge, pBlockedTags, pPINHash, pEnabled)
	p.Schedule = scheduleFromJSON(p.ID, scheduleJSON)
	return p, nil
}

func (r *ProfileRepository) GetByUserID(ctx context.Context, userID shared.UserID) ([]*user.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.GetByUserID")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at, last_active_at, access_schedule, schedule_override_until
		 FROM profiles WHERE user_id = $1 ORDER BY is_default DESC, name`, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("ProfileRepo.GetByUserID: %w", err)
	}
	defer rows.Close()

	var result []*user.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("ProfileRepo.GetByUserID scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("ProfileRepo.GetByUserID rows: %w", err)
	}
	return result, nil
}

func (r *ProfileRepository) Update(ctx context.Context, p *user.Profile) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.Update")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(p.ID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	prefsJSON, err := json.Marshal(p.Preferences)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE profiles SET name=$1, avatar_path=$2, preferences=$3, max_rating=$4, is_default=$5, is_kids=$6, audio_languages=$7, subtitle_languages=$8, max_bitrate_kbps=$9, parental_max_age=$10, parental_blocked_tags=$11, parental_pin_hash=$12, parental_enabled=$13, locale=$14, region=$15 WHERE id=$16`,
		p.Name, p.AvatarPath, prefsJSON, p.MaxRating, p.IsDefault, p.IsKids,
		pq.Array(audioLangs), pq.Array(subLangs), p.MaxBitrateKbps,
		pMaxAge, pq.Array(pBlockedTags), pPINHash, pEnabled,
		locale, p.Region, p.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("update profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

// UpdateAvatarPath writes ONLY avatar_path. Narrow by design: the previous
// avatar upload flow re-saved the WHOLE row (via Update) from a Profile it
// had read earlier, so a concurrent parental-settings write landing in
// between was silently erased - including a PIN just set by another request.
// Scoping the SQL to the single column that actually changed makes that
// clobber structurally impossible, with no need for a version/updated_at
// column (profiles has none, and adding one is a migration out of scope here).
func (r *ProfileRepository) UpdateAvatarPath(ctx context.Context, id shared.ProfileID, avatarPath *string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.UpdateAvatarPath")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE profiles SET avatar_path=$1 WHERE id=$2`, avatarPath, string(id),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("update profile avatar path: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

// TouchLastActive writes ONLY last_active_at, on the same column-scoped
// principle as UpdateAvatarPath: it runs on every token refresh and must never
// be able to replay a stale snapshot of any other column.
func (r *ProfileRepository) TouchLastActive(ctx context.Context, id shared.ProfileID, at time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.TouchLastActive")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE profiles SET last_active_at=$1 WHERE id=$2`, at, string(id),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("ProfileRepo.TouchLastActive: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// UpdateParentalSettings writes ONLY the parental_* columns - the mirror
// image of UpdateAvatarPath. Without this, saving a PIN/age/tags/enabled
// change from a Profile read before a concurrent avatar upload landed would
// silently revert that avatar_path back to its stale value.
func (r *ProfileRepository) UpdateParentalSettings(ctx context.Context, id shared.ProfileID, settings *user.ParentalSettings) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.UpdateParentalSettings")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pMaxAge, pBlockedTags, pPINHash, pEnabled := parentalToColumns(settings)
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE profiles SET parental_max_age=$1, parental_blocked_tags=$2, parental_pin_hash=$3, parental_enabled=$4 WHERE id=$5`,
		pMaxAge, pq.Array(pBlockedTags), pPINHash, pEnabled, string(id),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("update profile parental settings: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *ProfileRepository) SetDefault(ctx context.Context, userID shared.UserID, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.SetDefault")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", string(userID)),
		attribute.String("profile.id", string(profileID)),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("ProfileRepo.SetDefault begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && rbErr != sql.ErrTxDone {
				slog.Warn("ProfileRepo.SetDefault rollback failed", slog.Any("err", rbErr))
			}
		}
	}()
	if _, err = tx.ExecContext(ctx,
		`UPDATE profiles SET is_default=false WHERE user_id=$1 AND is_default=true AND id <> $2`,
		userID, profileID,
	); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("clear other defaults: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE profiles SET is_default=true WHERE id=$1 AND user_id=$2`,
		profileID, userID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("set default: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		err = shared.ErrNotFound
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err = tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (r *ProfileRepository) GetAll(ctx context.Context) ([]*user.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.GetAll")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, user_id, name, avatar_path, preferences, max_rating, is_default, is_kids, audio_languages, subtitle_languages, max_bitrate_kbps, parental_max_age, parental_blocked_tags, parental_pin_hash, parental_enabled, locale, region, created_at, last_active_at, access_schedule, schedule_override_until
		 FROM profiles ORDER BY user_id, is_default DESC, name LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("ProfileRepo.GetAll: %w", err)
	}
	defer rows.Close()

	var result []*user.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("ProfileRepo.GetAll scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("ProfileRepo.GetAll rows: %w", err)
	}
	return result, nil
}

func (r *ProfileRepository) Delete(ctx context.Context, id shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM profiles WHERE id = $1`, string(id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("delete profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
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
	var scheduleJSON []byte
	if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.AvatarPath, &prefsJSON, &p.MaxRating, &p.IsDefault, &p.IsKids,
		pq.Array(&p.AudioLanguages), pq.Array(&p.SubtitleLanguages), &p.MaxBitrateKbps,
		&pMaxAge, pq.Array(&pBlockedTags), &pPINHash, &pEnabled,
		&p.Locale, &p.Region, &p.CreatedAt, &p.LastActiveAt, &scheduleJSON, &p.ScheduleOverrideUntil); err != nil {
		return nil, fmt.Errorf("scanProfile: %w", err)
	}
	if err := json.Unmarshal(prefsJSON, &p.Preferences); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "preferences"), slog.Any("err", err))
	}
	p.Parental = parentalFromColumns(pMaxAge, pBlockedTags, pPINHash, pEnabled)
	p.Schedule = scheduleFromJSON(p.ID, scheduleJSON)
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

// UpdateSchedule writes ONLY access_schedule (nil clears it), column-scoped
// like UpdateParentalSettings.
func (r *ProfileRepository) UpdateSchedule(ctx context.Context, id shared.ProfileID, s *user.AccessSchedule) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.UpdateSchedule")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var raw []byte
	if s != nil {
		b, err := json.Marshal(scheduleToDTO(*s))
		if err != nil {
			return fmt.Errorf("ProfileRepo.UpdateSchedule marshal: %w", err)
		}
		raw = b
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE profiles SET access_schedule=$1 WHERE id=$2`, raw, string(id))
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ProfileRepo.UpdateSchedule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// SetScheduleOverride writes ONLY schedule_override_until (nil clears it).
func (r *ProfileRepository) SetScheduleOverride(ctx context.Context, id shared.ProfileID, until *time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.SetScheduleOverride")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE profiles SET schedule_override_until=$1 WHERE id=$2`, until, string(id))
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("ProfileRepo.SetScheduleOverride: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// scheduleDTO is the stored JSON shape of an AccessSchedule.
type scheduleDTO struct {
	Enabled bool              `json:"enabled"`
	Mode    string            `json:"mode"`
	Rules   []scheduleRuleDTO `json:"rules"`
}

type scheduleRuleDTO struct {
	Days   []int         `json:"days"`
	Ranges []scheduleRng `json:"ranges"`
}

type scheduleRng struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func scheduleToDTO(s user.AccessSchedule) scheduleDTO {
	out := scheduleDTO{Enabled: s.Enabled, Mode: string(s.Mode), Rules: make([]scheduleRuleDTO, 0, len(s.Rules))}
	for _, r := range s.Rules {
		rd := scheduleRuleDTO{Days: make([]int, 0, len(r.Days)), Ranges: make([]scheduleRng, 0, len(r.Ranges))}
		for _, d := range r.Days {
			rd.Days = append(rd.Days, int(d))
		}
		for _, tr := range r.Ranges {
			rd.Ranges = append(rd.Ranges, scheduleRng(tr))
		}
		out.Rules = append(out.Rules, rd)
	}
	return out
}

// scheduleFromJSON decodes a stored schedule. A row that does not decode or
// validate becomes an enabled schedule with no rule - closed, never open:
// a corrupt kids schedule must not unlock the profile (fail-closed).
func scheduleFromJSON(id shared.ProfileID, raw []byte) *user.AccessSchedule {
	if len(raw) == 0 {
		return nil
	}
	var dto scheduleDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		slog.Error("profile access schedule unreadable, profile kept closed", slog.String("profile_id", string(id)), slog.Any("err", err))
		return &user.AccessSchedule{Enabled: true, Mode: user.ScheduleModeWeek}
	}
	s := user.AccessSchedule{Enabled: dto.Enabled, Mode: user.ScheduleMode(dto.Mode), Rules: make([]user.ScheduleRule, 0, len(dto.Rules))}
	for _, rd := range dto.Rules {
		r := user.ScheduleRule{}
		for _, d := range rd.Days {
			r.Days = append(r.Days, time.Weekday(d))
		}
		for _, tr := range rd.Ranges {
			r.Ranges = append(r.Ranges, user.TimeRange(tr))
		}
		s.Rules = append(s.Rules, r)
	}
	if err := s.Validate(); err != nil {
		slog.Error("profile access schedule invalid, profile kept closed", slog.String("profile_id", string(id)), slog.Any("err", err))
		return &user.AccessSchedule{Enabled: true, Mode: user.ScheduleModeWeek}
	}
	return &s
}

// ListAdminProfiles returns profiles whose user has is_admin=TRUE.
// Used by notification service to route security alerts.
func (r *ProfileRepository) ListAdminProfiles(ctx context.Context) ([]*user.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.profile.ListAdminProfiles")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT p.id, p.user_id, p.name, p.avatar_path, p.preferences, p.max_rating, p.is_default, p.is_kids,
		 p.audio_languages, p.subtitle_languages, p.max_bitrate_kbps, p.parental_max_age, p.parental_blocked_tags,
		 p.parental_pin_hash, p.parental_enabled, p.locale, p.region, p.created_at, p.last_active_at, p.access_schedule, p.schedule_override_until
		 FROM profiles p
		 INNER JOIN users u ON u.id = p.user_id
		 WHERE u.is_admin = TRUE
		 ORDER BY p.user_id, p.is_default DESC, p.name LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("ProfileRepo.ListAdminProfiles: %w", err)
	}
	defer rows.Close()
	var result []*user.Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("ProfileRepo.ListAdminProfiles scan: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("ProfileRepo.ListAdminProfiles rows: %w", err)
	}
	return result, nil
}

var _ user.ProfileRepository = (*ProfileRepository)(nil)
