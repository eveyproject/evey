package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type InviteLinkRepository struct {
	db *sql.DB
}

func NewInviteLinkRepository(db *sql.DB) *InviteLinkRepository {
	return &InviteLinkRepository{db: db}
}

func (r *InviteLinkRepository) Create(ctx context.Context, link *user.InviteLink) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.invite_link.Create")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	libIDs := link.LibraryIDs
	if libIDs == nil {
		libIDs = []shared.LibraryID{}
	}
	libJSON, err := json.Marshal(libIDs)
	if err != nil {
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO invite_links (id, created_by, token, label, max_uses, used_count, is_admin, library_ids, expires_at, created_at, max_profiles)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		link.ID, link.CreatedBy, link.Token, link.Label, link.MaxUses, link.UsedCount, link.IsAdmin, libJSON, link.ExpiresAt, link.CreatedAt, link.MaxProfiles,
	)
	return err
}

func scanInviteLink(scanner interface {
	Scan(dest ...interface{}) error
}) (*user.InviteLink, error) {
	link := &user.InviteLink{}
	var libJSON []byte
	var maxProfiles sql.NullInt64
	if err := scanner.Scan(&link.ID, &link.CreatedBy, &link.Token, &link.Label, &link.MaxUses, &link.UsedCount, &link.IsAdmin, &libJSON, &link.ExpiresAt, &link.CreatedAt, &maxProfiles); err != nil {
		return nil, err
	}
	if maxProfiles.Valid {
		mp := int(maxProfiles.Int64)
		link.MaxProfiles = &mp
	}
	if len(libJSON) > 0 {
		if err := json.Unmarshal(libJSON, &link.LibraryIDs); err != nil {
			// Poisoned row: fail loudly so the admin can re-create the invite,
			// rather than silently stripping library grants on registration.
			return nil, fmt.Errorf("invite_link %s: unmarshal library_ids: %w", link.ID, err)
		}
	}
	return link, nil
}

func (r *InviteLinkRepository) GetByToken(ctx context.Context, token string) (*user.InviteLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.invite_link.GetByToken")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, created_by, token, label, max_uses, used_count, is_admin, library_ids, expires_at, created_at, max_profiles
		 FROM invite_links WHERE token = $1`, token)
	return scanInviteLink(row)
}

func (r *InviteLinkRepository) ListByCreator(ctx context.Context, creatorID string) ([]*user.InviteLink, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.invite_link.ListByCreator")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, created_by, token, label, max_uses, used_count, is_admin, library_ids, expires_at, created_at, max_profiles
		 FROM invite_links WHERE created_by = $1 ORDER BY created_at LIMIT 500`, creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*user.InviteLink
	for rows.Next() {
		link, err := scanInviteLink(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, rows.Err()
}

// DecrementUsedCount rolls back a previously-claimed use slot. Guarded to
// never go below 0. Used by registration flow when user.Create fails after a
// successful claim to avoid leaking a slot.
func (r *InviteLinkRepository) DecrementUsedCount(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.invite_link.DecrementUsedCount")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE invite_links SET used_count = used_count - 1
		 WHERE id = $1 AND used_count > 0`, id)
	return err
}

// IncrementUsedCount atomically increments used_count iff the invite still has
// remaining uses (max_uses == 0 → unlimited). Returns shared.ErrNotFound if the
// invite row is missing or already fully used. This is intentionally atomic
// (single UPDATE with conditional WHERE) to close the TOCTOU window between a
// caller's IsUsable() check and the actual increment: N concurrent claimers
// on a MaxUses=1 invite would previously all succeeded; now exactly one wins.
//
// Binds to an ambient *sql.Tx via txFromCtx so RegisterWithInvite can claim
// the slot, create the user, grant libraries and create the default profile
// in a single tx - any failure rolls back the claim atomically.
func (r *InviteLinkRepository) IncrementUsedCount(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.invite_link.IncrementUsedCount")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE invite_links SET used_count = used_count + 1
		 WHERE id = $1 AND (max_uses = 0 OR used_count < max_uses)`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *InviteLinkRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.invite_link.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM invite_links WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

var _ user.InviteLinkRepository = (*InviteLinkRepository)(nil)
