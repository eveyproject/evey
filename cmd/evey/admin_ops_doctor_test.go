package main

import (
	"os"
	"syscall"
	"testing"
)

// The doctor's media check must judge writability the way the kernel does -
// owner OR group OR other - against the SERVER's identity (PID 1), not just
// compare owner uids: a gravity:users library with g+rw is perfectly
// repairable by a server running as 1000:100 (the prod scheme), and the old
// owner-only check told the operator to chown it away.
func TestWritableByServer(t *testing.T) {
	st := func(uid, gid uint32) *syscall.Stat_t { return &syscall.Stat_t{Uid: uid, Gid: gid} }
	for _, tc := range []struct {
		name   string
		st     *syscall.Stat_t
		mode   os.FileMode
		uid    int
		gid    int
		groups []int
		want   bool
	}{
		{"owner with write bit", st(1000, 1000), 0o755, 1000, 1000, nil, true},
		{"owner without write bit", st(1000, 1000), 0o455, 1000, 1000, nil, false},
		{"other owner, primary gid matches g+w", st(1001, 100), 0o664, 1000, 100, nil, true},
		{"other owner, supplementary gid matches g+w", st(1001, 100), 0o664, 1000, 1000, []int{100, 105}, true},
		{"group matches but no g+w", st(1001, 100), 0o644, 1000, 100, nil, false},
		{"no match, world-writable", st(0, 0), 0o777, 1000, 1000, nil, true},
		{"no match, no o+w", st(0, 0), 0o755, 1000, 1000, nil, false},
		{"owner match wins over missing group bit", st(1000, 999), 0o700, 1000, 100, nil, true},
	} {
		got := writableByServer(tc.st, tc.mode, tc.uid, tc.gid, tc.groups)
		if got != tc.want {
			t.Errorf("%s: writableByServer = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The doctor may run from `docker exec` (image default user = root since the
// PUID entrypoint), so the server's REAL identity comes from /proc/1/status.
func TestParseProcStatusIDs(t *testing.T) {
	status := "Name:\tevey\nUid:\t1000\t1000\t1000\t1000\nGid:\t100\t100\t100\t100\nGroups:\t100 105 \n"
	uid, gid, groups := parseProcStatusIDs(status, 1, 2)
	if uid != 1000 || gid != 100 {
		t.Errorf("uid/gid = %d/%d, want 1000/100", uid, gid)
	}
	if len(groups) != 2 || groups[0] != 100 || groups[1] != 105 {
		t.Errorf("groups = %v, want [100 105]", groups)
	}

	uid, gid, groups = parseProcStatusIDs("garbage", 42, 43)
	if uid != 42 || gid != 43 || groups != nil {
		t.Errorf("fallback = %d/%d/%v, want 42/43/nil", uid, gid, groups)
	}
}
