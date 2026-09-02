package postgres

// The repair journal, written into the existing audit_log.
//
// It goes there rather than into a table of its own for one reason: the audit
// log is already the place an operator looks when asking "what happened to my
// library, and who did it". Splitting media rewrites into a separate store
// would leave that question answered in two places, and the 2026-07-13 incident
// showed what happens when the answer is in neither - the audit log held the
// operator's page view of the repair screen and nothing about the 98 files the
// repair then rewrote.
//
// One row per file per rewrite, action "media_repair", with the whole record in
// details. That payload is what makes a repair reversible without a backup: it
// carries the shift that was applied and the track offsets that existed before,
// values that live nowhere else once a file has been rewritten.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// RepairAuditAction is the audit_log action every media rewrite carries. One
// value, so the whole history of the library's bytes is one filter away.
const RepairAuditAction = "media_repair"

// RepairAuditResource is the resource_type of those entries.
const RepairAuditResource = "media_file"

// RepairJournalRepository implements maintenance.RepairJournal on top of the
// audit log.
type RepairJournalRepository struct {
	audit *AuditLogRepository
}

// NewRepairJournalRepository builds the journal over an existing audit log.
func NewRepairJournalRepository(audit *AuditLogRepository) *RepairJournalRepository {
	return &RepairJournalRepository{audit: audit}
}

// RecordRepair writes one entry. The caller treats a failure as loud but not
// fatal, so this returns the error rather than swallowing it here.
func (r *RepairJournalRepository) RecordRepair(ctx context.Context, rec appmaintenance.RepairRecord) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.repair_journal.Record")
	defer span.End()

	// Round-tripping through JSON keeps the stored shape identical to the Go
	// struct's tags: whoever reads these rows in five years reads the same
	// field names the code writes today.
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("repair journal: encode record: %w", err)
	}
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil {
		return fmt.Errorf("repair journal: decode record: %w", err)
	}

	resourceID := rec.MediaID
	entry := &activity.AuditLogEntry{
		ID:           uuid.New().String(),
		Action:       RepairAuditAction,
		ResourceType: RepairAuditResource,
		Details:      details,
		CreatedAt:    time.Now(),
	}
	if resourceID != "" {
		entry.ResourceID = &resourceID
	}
	if err := r.audit.Create(ctx, entry); err != nil {
		return fmt.Errorf("repair journal: %w", err)
	}
	return nil
}

// RepairHistory returns what was done to one file, newest first. The filter is
// on the path inside the payload rather than on resource_id: a media item can
// be deleted and re-added with a new id, and the question "what happened to
// THIS file" must survive that.
func (r *RepairJournalRepository) RepairHistory(ctx context.Context, path string, limit int) ([]appmaintenance.RepairRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.repair_journal.History")
	defer span.End()
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.audit.db).QueryContext(ctx,
		`SELECT details FROM audit_log
		 WHERE action = $1 AND details->>'path' = $2
		 ORDER BY created_at DESC
		 LIMIT $3`, RepairAuditAction, path, limit)
	if err != nil {
		return nil, fmt.Errorf("repair journal: read history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []appmaintenance.RepairRecord
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("repair journal: scan entry: %w", err)
		}
		var rec appmaintenance.RepairRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			// One unreadable row must not hide the rest of a file's history -
			// which is the only thing standing between an operator and a
			// manual investigation.
			continue
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repair journal: read history: %w", err)
	}
	return out, nil
}

// ListRepairs implements maintenance.RepairLister: every entry in the window,
// newest first, optionally one batch kind. Filtered on created_at (indexed)
// and on the job inside the payload.
func (r *RepairJournalRepository) ListRepairs(ctx context.Context, q appmaintenance.RepairQuery) ([]appmaintenance.RepairRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.repair_journal.List")
	defer span.End()
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	where := "action = $1"
	args := []any{RepairAuditAction}
	if !q.Since.IsZero() {
		args = append(args, q.Since)
		where += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if !q.Until.IsZero() {
		args = append(args, q.Until)
		where += fmt.Sprintf(" AND created_at <= $%d", len(args))
	}
	if q.Job != "" {
		args = append(args, q.Job)
		where += fmt.Sprintf(" AND details->>'job' = $%d", len(args))
	}
	args = append(args, limit)
	rows, err := txFromCtx(ctx, r.audit.db).QueryContext(ctx,
		fmt.Sprintf(`SELECT details FROM audit_log WHERE %s ORDER BY created_at DESC LIMIT $%d`, where, len(args)),
		args...)
	if err != nil {
		return nil, fmt.Errorf("repair journal: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []appmaintenance.RepairRecord
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("repair journal: scan entry: %w", err)
		}
		var rec appmaintenance.RepairRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue // one unreadable row must not hide the batch
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repair journal: list: %w", err)
	}
	return out, nil
}
