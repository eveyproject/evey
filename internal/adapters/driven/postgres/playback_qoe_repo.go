package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// PlaybackQoERepo writes Common Media Client Data (CTA-5004) events received
// from segment requests. Reads (admin dashboard percentiles) live in a
// separate reader to keep responsibilities split.
//
// Phase 1 = ingest only; no SELECTs here yet. Phase 2 adds the reader.
type PlaybackQoERepo struct {
	db *sql.DB
}

func NewPlaybackQoERepo(db *sql.DB) *PlaybackQoERepo {
	return &PlaybackQoERepo{db: db}
}

// QoEEventRow is the storage projection - exposed so the app-layer sink can
// hand us a fully-formed row without us pulling in the domain CMCD parser.
// Pointer fields stay null when the player didn't send that key.
type QoEEventRow struct {
	UserID           *string
	MediaItemID      *string
	SessionID        string
	ContentID        string
	VariantIdx       *int
	SegmentIdx       *int
	ObjectType       string
	ObjectDurationMs *int
	BufferLengthMs   *int
	EncodedKbps      *int
	TopBitrateKbps   *int
	MeasuredKbps     *int
	RequestedMaxKbps *int
	DeadlineMs       *int
	PlaybackRate     *float64
	StreamingFormat  string
	StreamType       string
	CMCDVersion      *int
	BufferStarved    bool
	Startup          bool
	Raw              map[string]string
	DeviceClass      string
	RemoteIP         string
}

// qoeInsertColumns is the column list every QoE insert fills, in the order
// qoeRowArgs lays the values out.
const qoeInsertColumns = `(
			user_id, media_item_id,
			session_id, content_id,
			variant_idx, segment_idx,
			object_type, object_duration_ms,
			buffer_length_ms, encoded_kbps, top_bitrate_kbps, measured_kbps,
			requested_max_kbps, deadline_ms, playback_rate,
			streaming_format, stream_type, cmcd_version,
			buffer_starved, startup,
			raw, device_class, remote_ip
		)`

// qoeColumnsPerRow is the width of one VALUES tuple; InsertMany bounds its
// batches so the statement stays under the driver's parameter limit.
const qoeColumnsPerRow = 23

// qoeMaxRowsPerStatement keeps one INSERT well under the 65535 parameters a
// statement may carry.
const qoeMaxRowsPerStatement = 1000

// qoeRowArgs lays one row out as the arguments of its VALUES tuple.
func qoeRowArgs(evt QoEEventRow) []any {
	rawJSON, err := json.Marshal(evt.Raw)
	if err != nil {
		// Should be unreachable for map[string]string; fall back to "{}" so
		// the row still inserts.
		rawJSON = []byte("{}")
	}
	return []any{
		evt.UserID, evt.MediaItemID,
		nullIfEmpty(evt.SessionID), nullIfEmpty(evt.ContentID),
		evt.VariantIdx, evt.SegmentIdx,
		nullIfEmpty(evt.ObjectType), evt.ObjectDurationMs,
		evt.BufferLengthMs, evt.EncodedKbps, evt.TopBitrateKbps, evt.MeasuredKbps,
		evt.RequestedMaxKbps, evt.DeadlineMs, evt.PlaybackRate,
		nullIfEmpty(evt.StreamingFormat), nullIfEmpty(evt.StreamType), evt.CMCDVersion,
		evt.BufferStarved, evt.Startup,
		rawJSON, nullIfEmpty(evt.DeviceClass), evt.RemoteIP,
	}
}

// qoeValuesTuple renders the placeholders of the row whose arguments start at
// the 1-based index first.
func qoeValuesTuple(first int) string {
	var b strings.Builder
	b.WriteByte('(')
	for i := range qoeColumnsPerRow {
		if i > 0 {
			b.WriteString(", ")
		}
		if i == qoeColumnsPerRow-1 {
			fmt.Fprintf(&b, "NULLIF($%d, '')::inet", first+i)
			continue
		}
		fmt.Fprintf(&b, "$%d", first+i)
	}
	b.WriteByte(')')
	return b.String()
}

// Insert appends one CMCD event. Errors propagate; the caller (sink worker)
// is responsible for swallowing them - a failed write must never bubble up
// to the segment handler.
func (r *PlaybackQoERepo) Insert(ctx context.Context, evt QoEEventRow) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.qoe.Insert")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	q := "INSERT INTO playback_qoe_events " + qoeInsertColumns + " VALUES " + qoeValuesTuple(1)
	if _, err := txFromCtx(ctx, r.db).ExecContext(ctx, q, qoeRowArgs(evt)...); err != nil {
		return fmt.Errorf("playback_qoe insert: %w", err)
	}
	return nil
}

// InsertMany appends the events in as few statements as the parameter limit
// allows: the sink hands over what piled up behind a slow write, and one
// round trip per segment and per viewer is what made the write slow.
func (r *PlaybackQoERepo) InsertMany(ctx context.Context, evts []QoEEventRow) error {
	if len(evts) == 0 {
		return nil
	}
	ctx, span := tracing.StartSpan(ctx, "postgres.qoe.InsertMany")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	for start := 0; start < len(evts); start += qoeMaxRowsPerStatement {
		chunk := evts[start:min(start+qoeMaxRowsPerStatement, len(evts))]
		var q strings.Builder
		q.WriteString("INSERT INTO playback_qoe_events ")
		q.WriteString(qoeInsertColumns)
		q.WriteString(" VALUES ")
		args := make([]any, 0, len(chunk)*qoeColumnsPerRow)
		for i, evt := range chunk {
			if i > 0 {
				q.WriteString(", ")
			}
			q.WriteString(qoeValuesTuple(len(args) + 1))
			args = append(args, qoeRowArgs(evt)...)
		}
		if _, err := txFromCtx(ctx, r.db).ExecContext(ctx, q.String(), args...); err != nil {
			return fmt.Errorf("playback_qoe insert many (%d rows): %w", len(chunk), err)
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Purge deletes CMCD events recorded before the cutoff, returning how many rows
// went. This table is the busiest write path in the server - one row per
// CMCD-carrying segment request, unsampled, each with a JSONB payload and four
// indexes - and it is append-only otherwise, so this sweep is the ONLY thing
// bounding its growth.
//
// Deletes by ts, which idx_qoe_ts covers. Migration 000103 sized its indexes
// for a 30-day window and said so in a
// comment; the sweep that comment assumed was never written, so the table grew
// without limit from day one.
func (r *PlaybackQoERepo) Purge(ctx context.Context, before time.Time) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.qoe.Purge")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM playback_qoe_events WHERE ts < $1`, before)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
