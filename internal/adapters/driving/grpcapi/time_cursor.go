package grpcapi

import (
	"fmt"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// timeCursor is the decoded form of an opaque cursor for a time-ordered
// feed (play history, login history, watch history). All such feeds
// share the same (timestamp, id) DESC keyset, so one helper handles all
// of them.
type timeCursor struct {
	t  time.Time
	id string
}

// encodeTimeCursor serializes a (timestamp, id) keyset as an opaque
// shared.Cursor with SortField="started_at" - the client never needs to
// interpret it.
func encodeTimeCursor(t time.Time, id string) string {
	return shared.EncodeCursor(shared.Cursor{
		SortField: "started_at",
		SortValue: t.UTC().Format(time.RFC3339Nano),
		LastID:    id,
	})
}

// decodeTimeCursor returns the parsed (timestamp, id) pair. Empty input
// is an error - callers should branch on req.Cursor != "" before
// calling this.
func decodeTimeCursor(s string) (timeCursor, error) {
	c, err := shared.DecodeCursor(s)
	if err != nil {
		return timeCursor{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, c.SortValue)
	if err != nil {
		return timeCursor{}, fmt.Errorf("parse cursor timestamp: %w", err)
	}
	return timeCursor{t: t, id: c.LastID}, nil
}
