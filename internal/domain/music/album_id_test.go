package music

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestAlbumFolderHash_MatchesCoverArtFormula(t *testing.T) {
	// The album folder hash must equal sha256(folder + "|") so it stays
	// consistent with the on-disk cover-art addressing.
	folder := "/media4/Neon Harbor - From Nowhere (Deluxe Edition)"
	sum := sha256.Sum256([]byte(folder + "|"))
	want := hex.EncodeToString(sum[:])
	if got := AlbumFolderHash(folder); got != want {
		t.Fatalf("AlbumFolderHash = %s, want %s", got, want)
	}
}

func TestAlbumID_ShortAndStable(t *testing.T) {
	folder := "/media4/Neon Harbor - From Nowhere (Deluxe Edition)"
	id := AlbumID(folder)
	if len(id) != 16 {
		t.Fatalf("AlbumID length = %d, want 16 (%q)", len(id), id)
	}
	if AlbumID(folder) != id {
		t.Fatal("AlbumID not stable across calls")
	}
	// It's the prefix of the full folder hash.
	if AlbumFolderHash(folder)[:16] != id {
		t.Fatalf("AlbumID %q is not the 16-char prefix of the folder hash", id)
	}
}

func TestAlbumID_DistinguishesHomonymFolders(t *testing.T) {
	// Two different folders that share the same (artist, album) display tags
	// must still get distinct ids - the whole point of folder-based identity.
	a := AlbumID("/musicA/Various - Greatest Hits")
	b := AlbumID("/musicB/Various - Greatest Hits")
	if a == b {
		t.Fatalf("homonym albums collided on id: %s", a)
	}
}
