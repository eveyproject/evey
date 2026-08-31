package sysinfo

import (
	"strings"
	"testing"
)

const fakeMounts = `overlay / overlay rw,relatime 0 0
proc /proc proc rw 0 0
/dev/sda1 /data ext4 rw,relatime 0 0
mergerfs /media/pool fuse.mergerfs rw,relatime,user_id=0 0 0
/dev/sdb1 /media/pool-backup ext4 rw 0 0
nas:/export /mnt/nas nfs4 rw,vers=4.2 0 0
C:\134 /mnt/c 9p rw,dirsync 0 0
/dev/sdc1 /media/my\040films ext4 rw 0 0
`

func TestMountFSType_LongestPrefixWins(t *testing.T) {
	entries := parseMountTable(strings.NewReader(fakeMounts))
	cases := map[string]string{
		"/media/pool/Series/ep.mkv": "fuse.mergerfs",
		"/media/pool":               "fuse.mergerfs",
		"/media/pool-backup/x.mkv":  "ext4",
		"/mnt/nas/film.mkv":         "nfs4",
		"/mnt/c/Users/x.mkv":        "9p",
		"/data/cache/seg.ts":        "ext4",
		"/media/my films/a.mkv":     "ext4",
		"/somewhere/else":           "overlay",
	}
	for path, want := range cases {
		if got := mountFSType(entries, path); got != want {
			t.Errorf("mountFSType(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestMountFSType_NoTable_IsEmpty(t *testing.T) {
	if got := mountFSType(nil, "/media/x"); got != "" {
		t.Fatalf("no mount table must yield %q, got %q", "", got)
	}
}
