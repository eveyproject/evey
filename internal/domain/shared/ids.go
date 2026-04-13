package shared

// Typed identifiers for compile-time safety against ID mix-ups (e.g. passing a
// LibraryID where a UserID is expected). Conversion to/from string at adapter
// boundaries (HTTP/gRPC/SQL) requires explicit cast: UserID(s) / string(id).
type (
	UserID      string
	ProfileID   string
	LibraryID   string
	MediaItemID string
)
