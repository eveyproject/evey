package grpcapi

import (
	"encoding/json"
	"fmt"
	"time"

	photodomain "github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// photoCursorSortValue is the payload packed into shared.Cursor.SortValue for
// photo listings. The keyset needs (taken_at, created_at, id) - one field more
// than shared.Cursor's single SortValue slot carries on its own - so the extra
// field rides along as JSON inside that slot instead of a second cursor
// format.
type photoCursorSortValue struct {
	TakenAt   string `json:"t,omitempty"`
	CreatedAt string `json:"c"`
}

// encodePhotoCursor builds the opaque cursor pointing to the page after last,
// the final photo of the current page. Mirrors the taken_at DESC NULLS LAST,
// created_at DESC, id DESC keyset order that photo.PhotoCursor seeks on.
func encodePhotoCursor(last photodomain.Photo) string {
	v := photoCursorSortValue{CreatedAt: last.CreatedAt.UTC().Format(time.RFC3339Nano)}
	if last.TakenAt != nil {
		v.TakenAt = last.TakenAt.UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(v)
	if err != nil {
		// Unreachable with a struct of strings, but an empty cursor is the
		// honest failure here: the client reads it as end-of-list and stops,
		// rather than paging on with a corrupt keyset and silently skipping rows.
		return ""
	}
	return shared.EncodeCursor(shared.Cursor{
		SortField: "taken_at",
		SortValue: string(b),
		LastID:    last.ID,
	})
}

// decodePhotoCursor parses an opaque ListPhotosRequest.cursor back into the
// keyset the repository seeks from. Empty input is an error - callers must
// branch on req.Cursor != "" before calling this.
func decodePhotoCursor(s string) (*photodomain.PhotoCursor, error) {
	c, err := shared.DecodeCursor(s)
	if err != nil {
		return nil, err
	}
	var v photoCursorSortValue
	if uErr := json.Unmarshal([]byte(c.SortValue), &v); uErr != nil {
		return nil, fmt.Errorf("unmarshal photo cursor: %w", uErr)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, v.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse photo cursor created_at: %w", err)
	}
	cursor := &photodomain.PhotoCursor{CreatedAt: createdAt, ID: c.LastID}
	if v.TakenAt != "" {
		takenAt, err := time.Parse(time.RFC3339Nano, v.TakenAt)
		if err != nil {
			return nil, fmt.Errorf("parse photo cursor taken_at: %w", err)
		}
		cursor.TakenAt = &takenAt
	}
	return cursor, nil
}
