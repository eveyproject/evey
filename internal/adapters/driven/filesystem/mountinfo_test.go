package filesystem

import (
	"strings"
	"testing"
)

// A realistic mount table: root, a nested local disk, an SMB share, and an NFS
// share whose mount point is a prefix of an unrelated local one.
const sampleMounts = `/dev/sda1 / ext4 rw,relatime 0 0
/dev/sdb1 /media/local ext4 rw,relatime 0 0
//nas/films /media/nas cifs rw,vers=3.0 0 0
nas:/export/tv /media/tv nfs4 rw 0 0
/dev/sdc1 /media/nas-backup ext4 rw 0 0
tmpfs /tmp tmpfs rw 0 0
`

func TestFsTypeFor_LongestPrefixWins(t *testing.T) {
	entries := parseMounts(strings.NewReader(sampleMounts))

	cases := []struct {
		path string
		want string
	}{
		// A file deep inside a share resolves to that share, not to /.
		{"/media/nas/Films/Some Movie.mkv", "cifs"},
		{"/media/tv/Show/S01E01.mkv", "nfs4"},
		{"/media/local/movie.mkv", "ext4"},
		// The mount point itself.
		{"/media/nas", "cifs"},
		// Falls back to root when nothing more specific matches.
		{"/home/user/file.mkv", "ext4"},
	}
	for _, tc := range cases {
		got, ok := fsTypeFor(entries, tc.path)
		if !ok || got != tc.want {
			t.Errorf("fsTypeFor(%q) = %q (ok=%v), want %q", tc.path, got, ok, tc.want)
		}
	}
}

// /media/nas-backup must NOT be treated as living under /media/nas. A substring
// match here would misclassify a local disk as a network share and pin it to
// permanent frequent scanning.
func TestFsTypeFor_SiblingPrefixIsNotAMatch(t *testing.T) {
	entries := parseMounts(strings.NewReader(sampleMounts))
	got, _ := fsTypeFor(entries, "/media/nas-backup/Films/x.mkv")
	if got != "ext4" {
		t.Fatalf("a sibling path sharing a prefix must not match the share: got %q", got)
	}
}

func TestNetworkFSTypes_ClassifiesShares(t *testing.T) {
	network := []string{"nfs", "nfs4", "cifs", "smb3", "9p"}
	local := []string{"ext4", "xfs", "btrfs", "zfs", "overlay", "tmpfs"}

	for _, fs := range network {
		if !networkFSTypes[fs] {
			t.Errorf("%s must be treated as a network filesystem - inotify cannot see remote writes there", fs)
		}
	}
	for _, fs := range local {
		if networkFSTypes[fs] {
			t.Errorf("%s is local; classifying it as network would pin it to permanent scanning", fs)
		}
	}
}

// The kernel escapes characters that would break the space-separated format.
// Without decoding, a library under such a path never matches its mount entry
// and is silently misreported as local.
func TestParseMounts_DecodesEscapedMountPoints(t *testing.T) {
	entries := parseMounts(strings.NewReader(`//nas/x /media/My\040Films cifs rw 0 0` + "\n"))
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].point != "/media/My Films" {
		t.Fatalf("escaped mount point not decoded: %q", entries[0].point)
	}
	if got, _ := fsTypeFor(entries, "/media/My Films/a.mkv"); got != "cifs" {
		t.Fatalf("a path under an escaped mount point must resolve, got %q", got)
	}
}

func TestParseMounts_SkipsMalformedLines(t *testing.T) {
	entries := parseMounts(strings.NewReader("garbage\n/dev/sda1 / ext4 rw 0 0\n\n"))
	if len(entries) != 1 {
		t.Fatalf("malformed lines must be skipped, got %d entries", len(entries))
	}
}
