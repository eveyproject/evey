package mkvgo

import "testing"

func TestDecideSourceAccess(t *testing.T) {
	cases := []struct {
		name   string
		magic  int64
		fsType string
		want   bool
	}{
		{"ext4", 0xEF53, "ext4", false},
		{"mergerfs pool", fuseMagic, "fuse.mergerfs", false},
		{"fuse without a mountinfo line", fuseMagic, "", false},
		{"rclone", fuseMagic, "fuse.rclone", true},
		{"sshfs", fuseMagic, "fuse.sshfs", true},
		{"nfs", 0x6969, "nfs4", true},
		{"smb", 0xFE534D42, "cifs", true},
		{"cifs", 0xFF534D42, "cifs", true},
		{"9p drvfs", 0x01021997, "9p", true},
	}
	for _, c := range cases {
		if got := decideSourceAccess(c.magic, c.fsType); got != c.want {
			t.Errorf("%s: network = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMountFSType(t *testing.T) {
	mi := []byte("36 35 98:0 /mnt1 /mnt1 rw,noatime master:1 - ext3 /dev/root rw,errors=continue\n" +
		"40 35 0:52 / /srv/pool rw,relatime shared:5 - fuse.mergerfs pool rw,user_id=0\n")
	if got := mountFSType(mi, 0, 52); got != "fuse.mergerfs" {
		t.Errorf("0:52 = %q", got)
	}
	if got := mountFSType(mi, 98, 0); got != "ext3" {
		t.Errorf("98:0 = %q", got)
	}
	if got := mountFSType(mi, 1, 1); got != "" {
		t.Errorf("1:1 = %q", got)
	}
}
