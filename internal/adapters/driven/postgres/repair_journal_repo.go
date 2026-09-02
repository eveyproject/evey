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
