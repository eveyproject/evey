package shared

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Cursor encodes a keyset position for pagination: the sort field name,
// the last seen value of that field, and the row's ID as a tiebreaker.
// Wire format is opaque base64(JSON) so the server can change the schema
// later without breaking clients that round-trip it.
//
// Use with WHERE (sort_value, id) > (cursor.value, cursor.id) ORDER BY
// sort_value, id - the (value, id) tuple comparison guarantees no skips
// or duplicates even when sort_value is non-unique.
type Cursor struct {
	SortField string `json:"f"`
	SortValue string `json:"v"`
	LastID    string `json:"i"`
}

// ErrCursorMismatch is returned when the cursor was generated for a
// different sort key than the current request - re-issuing it would
// produce undefined ordering. Callers should restart from page 1.
var ErrCursorMismatch = errors.New("cursor sort field does not match request")

// EncodeCursor serializes a Cursor for the wire. Empty cursor → empty
// string (callers treat empty as "first page").
func EncodeCursor(c Cursor) string {
	if c.LastID == "" {
		return ""
	}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses an opaque cursor string. Empty input → zero
// Cursor, no error (the caller treats this as "first page"). Invalid
// base64 or JSON returns an error so the caller can 400 the request
// rather than silently restart.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, fmt.Errorf("unmarshal cursor: %w", err)
	}
	return c, nil
}
