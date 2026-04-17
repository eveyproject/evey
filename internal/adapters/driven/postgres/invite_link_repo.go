package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
)

type InviteLinkRepository struct {
	db *sql.DB
}

func NewInviteLinkRepository(db *sql.DB) *InviteLinkRepository {
	return &InviteLinkRepository{db: db}
}

func (r *InviteLinkRepository) Create(ctx context.Context, link *user.InviteLink) error {
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
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO invite_links (id, created_by, token, label, max_uses, used_count, is_admin, library_ids, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		link.ID, link.CreatedBy, link.Token, link.Label, link.MaxUses, link.UsedCount, link.IsAdmin, libJSON, link.ExpiresAt, link.CreatedAt,
	)
	return err
}

func scanInviteLink(scanner interface {
	Scan(dest ...interface{}) error
}) (*user.InviteLink, error) {
	link := &user.InviteLink{}
	var libJSON []byte
	if err := scanner.Scan(&link.ID, &link.CreatedBy, &link.Token, &link.Label, &link.MaxUses, &link.UsedCount, &link.IsAdmin, &libJSON, &link.ExpiresAt, &link.CreatedAt); err != nil {
		return nil, err
	}
	if len(libJSON) > 0 {
		if err := json.Unmarshal(libJSON, &link.LibraryIDs); err != nil {
			slog.Warn("invite_link: unmarshal library_ids", slog.String("id", link.ID), slog.Any("err", err))
		}
	}
	return link, nil
}

func (r *InviteLinkRepository) GetByToken(ctx context.Context, token string) (*user.InviteLink, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := r.db.QueryRowContext(ctx,
		`SELECT id, created_by, token, label, max_uses, used_count, is_admin, library_ids, expires_at, created_at
		 FROM invite_links WHERE token = $1`, token)
	return scanInviteLink(row)
}

func (r *InviteLinkRepository) ListByCreator(ctx context.Context, creatorID string) ([]*user.InviteLink, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, created_by, token, label, max_uses, used_count, is_admin, library_ids, expires_at, created_at
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

func (r *InviteLinkRepository) IncrementUsedCount(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE invite_links SET used_count = used_count + 1 WHERE id = $1`, id)
	return err
}

func (r *InviteLinkRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM invite_links WHERE id = $1`, id)
	return err
}

var _ user.InviteLinkRepository = (*InviteLinkRepository)(nil)
