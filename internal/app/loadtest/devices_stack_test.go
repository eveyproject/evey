//go:build linux

package loadtest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fakeSys stages the /sys entries of a Synology SHR stack (a volume dm-0 on
// an md array md2 over the partitions sda5, sdb5, sdc5) beside a plain disk
// sdd with an LVM volume dm-1 on its partition sdd3, as the kernel lays them
// out: /sys/block holds disks and volumes, /sys/class/block every device
// including partitions, slaves/ names what a volume sits on.
func fakeSys(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ln := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	mk("sys/block")
	mk("sys/class/block")
	for _, d := range []string{"sda", "sdb", "sdc", "sdd"} {
		mk("sys/devices/pci0/host0/block/" + d)
		ln("../devices/pci0/host0/block/"+d, "sys/block/"+d)
		ln("../../devices/pci0/host0/block/"+d, "sys/class/block/"+d)
	}
	for _, p := range []string{"sda/sda5", "sdb/sdb5", "sdc/sdc5", "sdd/sdd3"} {
		mk("sys/devices/pci0/host0/block/" + p)
		ln("../../devices/pci0/host0/block/"+p, "sys/class/block/"+filepath.Base(p))
	}
	for _, v := range []string{"md2", "dm-0", "dm-1"} {
		mk("sys/devices/virtual/block/" + v + "/slaves")
		ln("../devices/virtual/block/"+v, "sys/block/"+v)
		ln("../../devices/virtual/block/"+v, "sys/class/block/"+v)
	}
	slave := func(vol, of string) {
		ln("../../../../pci0/host0/block/"+of, "sys/devices/virtual/block/"+vol+"/slaves/"+filepath.Base(of))
	}
	slave("md2", "sda/sda5")
	slave("md2", "sdb/sdb5")
	slave("md2", "sdc/sdc5")
	ln("../../md2", "sys/devices/virtual/block/dm-0/slaves/md2")
	slave("dm-1", "sdd/sdd3")
	return root
}

// A volume is judged on the physical disks under it, through every level of
// the stack; a plain disk is its own.
func TestPhysicalDisks_WalksTheStack(t *testing.T) {
	root := fakeSys(t)
	for _, c := range []struct {
		dev  string
		want []string
	}{
		{"dm-0", []string{"sda", "sdb", "sdc"}}, // SHR: volume -> md2 -> partitions -> disks
		{"md2", []string{"sda", "sdb", "sdc"}},
		{"dm-1", []string{"sdd"}}, // LVM on a partition
		{"sdd3", []string{"sdd"}}, // a partition is its disk
		{"sda", []string{"sda"}},
		{"nope", nil},
	} {
		if got := physicalDisks(root, c.dev); !slices.Equal(got, c.want) {
			t.Errorf("%s -> %v, want %v", c.dev, got, c.want)
		}
	}
}

// When a read moved sectors on every level of a stack (btrfs on SHR: the
// anonymous st_dev leaves only the read), the location is the top of the
// stack, so its disks cover every member and not only the one a mirror read.
func TestStackTop_PicksTheVolume(t *testing.T) {
	root := fakeSys(t)
	for _, c := range []struct {
		movers []string
		want   string
	}{
		{[]string{"sda", "md2", "dm-0"}, "dm-0"},
		{[]string{"md2", "sdb"}, "md2"},
		{[]string{"sdd"}, "sdd"},
		{nil, ""},
	} {
		if got := stackTop(root, c.movers); got != c.want {
			t.Errorf("movers %v -> %q, want %q", c.movers, got, c.want)
		}
	}
	m := []DeviceMapping{{Location: "/volume1", Device: "dm-0", Disks: physicalDisks(root, "dm-0")}, {Location: "/volume2", Device: "dm-1", Disks: physicalDisks(root, "dm-1")}}
	if got := mappedDevices(m); !slices.Equal(got, []string{"sda", "sdb", "sdc", "sdd"}) {
		t.Fatalf("mapped = %v, want the four physical disks", got)
	}
}

// A location on a network share (or any volume this machine cannot see) is
// unresolved: the mapping is not complete, and the verdict must not claim
// that nothing on the storage gave out.
func TestLimiterOf_StorageNotMeasured(t *testing.T) {
	share := []DeviceMapping{{Location: "/mnt/nas"}}
	if l, _, _ := limiterOf(StopSLOBreach, "", ResourceStats{}, DiskVerdict{}, false, share); l != LimiterStorageUnmeasured {
		t.Fatalf("network share: limiter %q, want %q", l, LimiterStorageUnmeasured)
	}
	if l, _, _ := limiterOf(StopSLOBreach, "", ResourceStats{}, DiskVerdict{}, false, nil); l != LimiterStorageUnmeasured {
		t.Fatalf("no mapping at all: limiter %q, want %q", l, LimiterStorageUnmeasured)
	}
	mixed := []DeviceMapping{{Location: "/mnt/disk_1", Device: "sda", Disks: []string{"sda"}}, {Location: "/mnt/nas"}}
	if l, _, _ := limiterOf(StopSLOBreach, "", ResourceStats{}, DiskVerdict{Device: "sda"}, true, mixed); l != LimiterStorageUnmeasured {
		t.Fatalf("one location unmeasured, through the proxy: limiter %q, want the storage first", l)
	}
	// What was measured still speaks: a CPU at its ceiling is named even on a
	// share.
	if l, _, _ := limiterOf(StopSLOBreach, "", ResourceStats{CPUPercentPeak: 99}, DiskVerdict{}, false, share); l != LimiterCPU {
		t.Fatalf("cpu on a share: limiter %q, want cpu", l)
	}
}
