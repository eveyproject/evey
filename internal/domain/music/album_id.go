package music

import (
	"crypto/sha256"
	"encoding/hex"
)

// AlbumFolderHash returns the full sha256 hex of an album's folder identity.
// The album folder (not the (artist, album) tag tuple) is the album's identity,
// so two folders sharing the same tags stay distinct. This is the same value
// used to address cached cover art on disk.
func AlbumFolderHash(folder string) string {
	sum := sha256.Sum256([]byte(folder + "|"))
	return hex.EncodeToString(sum[:])
}

// AlbumID returns the short, opaque, URL-safe public identifier for an album,
// derived from its folder. Used in client URLs in place of the display name so
// homonym albums don't collide and paths aren't leaked. 16 hex chars (64 bits)
// is collision-safe within a library's album count.
func AlbumID(folder string) string {
	return AlbumFolderHash(folder)[:16]
}
