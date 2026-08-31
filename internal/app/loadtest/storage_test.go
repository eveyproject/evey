package loadtest

import "testing"

func TestLibraryStorage_FUSEPoolIsOpaque(t *testing.T) {
	cands := []MediaCandidate{
		{ID: "a", Path: "/media/pool/a.mkv"},
		{ID: "b", Path: "/media/pool/b.mkv"},
		{ID: "c", Path: ""}, // no path: nothing to inspect
	}
	fs, opaque := libraryStorage(cands, func(string) string { return "fuse.mergerfs" })
	if fs != "fuse.mergerfs" || !opaque {
		t.Fatalf("mergerfs pool: got (%q, %v), want (fuse.mergerfs, true)", fs, opaque)
	}
}

func TestLibraryStorage_LocalDiskIsNot(t *testing.T) {
	fs, opaque := libraryStorage([]MediaCandidate{{ID: "a", Path: "/media/a.mkv"}}, func(string) string { return "ext4" })
	if fs != "ext4" || opaque {
		t.Fatalf("ext4: got (%q, %v), want (ext4, false)", fs, opaque)
	}
}

func TestLibraryStorage_MixedMountsListedSortedAndOpaqueIfAny(t *testing.T) {
	byPath := map[string]string{"/a/x.mkv": "xfs", "/b/y.mkv": "nfs4", "/c/z.mkv": "xfs"}
	fs, opaque := libraryStorage([]MediaCandidate{{Path: "/a/x.mkv"}, {Path: "/b/y.mkv"}, {Path: "/c/z.mkv"}}, func(p string) string { return byPath[p] })
	if fs != "nfs4, xfs" || !opaque {
		t.Fatalf("mixed: got (%q, %v), want (\"nfs4, xfs\", true)", fs, opaque)
	}
}

func TestLibraryStorage_UnknownTableIsSilent(t *testing.T) {
	fs, opaque := libraryStorage([]MediaCandidate{{Path: "/a/x.mkv"}}, func(string) string { return "" })
	if fs != "" || opaque {
		t.Fatalf("no mount table: got (%q, %v), want (\"\", false) - never claim what was not seen", fs, opaque)
	}
}

func TestStorageOpaque_Families(t *testing.T) {
	for fs, want := range map[string]bool{
		"fuse.mergerfs": true, "fuse.rclone": true, "fuse": true, "9p": true, "virtiofs": true, "cifs": true,
		"ext4": false, "xfs": false, "btrfs": false, "zfs": false, "overlay": false, "": false,
	} {
		if got := storageOpaque(fs); got != want {
			t.Errorf("storageOpaque(%q) = %v, want %v", fs, got, want)
		}
	}
}
