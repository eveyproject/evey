package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeBlockDevice lays out /sys/block/<name> the way the kernel does, with
// the identity files given (empty value = file absent).
func fakeBlockDevice(t *testing.T, sysBlock, name string, slaves int, files map[string]string) {
	t.Helper()
	dir := filepath.Join(sysBlock, name)
	for _, sub := range []string{"slaves", "queue", "device"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i := range slaves {
		if err := os.WriteFile(filepath.Join(dir, "slaves", "sd"+string(rune('a'+i))), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for rel, v := range files {
		if v == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func useFakeProc(t *testing.T) (diskstats, netdev, sysBlock string) {
	t.Helper()
	root := t.TempDir()
	diskstats = filepath.Join(root, "diskstats")
	netdev = filepath.Join(root, "netdev")
	sysBlock = filepath.Join(root, "block")
	prevDisk, prevNet, prevSys, prevIO := procDiskstatsPath, procNetDevPath, sysBlockPath, ioPrev
	procDiskstatsPath, procNetDevPath, sysBlockPath = diskstats, netdev, sysBlock
	ioPrev = diskSnapshot{}
	t.Cleanup(func() {
		procDiskstatsPath, procNetDevPath, sysBlockPath, ioPrev = prevDisk, prevNet, prevSys, prevIO
	})
	return
}

const netDevHeader = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
`

// Two readings a second apart: sda served 100 reads that each waited 30 ms
// and was busy 60 % of the time; the partition line, the loop device and the
// md volume made of other disks must not appear.
func TestSampleIO_DiffsWholePhysicalDisksOnly(t *testing.T) {
	diskstats, netdev, sysBlock := useFakeProc(t)
	fakeBlockDevice(t, sysBlock, "sda", 0, map[string]string{
		"device/model":     "QEMU HARDDISK   ",
		"device/wwid":      "t10.QEMU    QEMU HARDDISK   drive-scsi1",
		"queue/rotational": "1",
	})
	fakeBlockDevice(t, sysBlock, "sdb", 0, map[string]string{
		"device/model":     "QEMU HARDDISK",
		"device/wwid":      "t10.QEMU    QEMU HARDDISK   drive-scsi2",
		"queue/rotational": "0",
	})
	fakeBlockDevice(t, sysBlock, "md0", 2, nil)
	fakeBlockDevice(t, sysBlock, "loop0", 0, nil)

	// major minor name reads merged sectors read_ms writes merged sectors write_ms inflight io_ticks weighted
	writeFixture(t, diskstats, `   8       0 sda 1000 0 20000 5000 500 0 8000 1000 0 10000 6000 0 0 0 0 0 0
   8       1 sda1 1000 0 20000 5000 500 0 8000 1000 0 10000 6000 0 0 0 0 0 0
   8      16 sdb 10 0 80 10 0 0 0 0 0 20 10 0 0 0 0 0 0
   9       0 md0 10 0 80 10 0 0 0 0 0 20 10 0 0 0 0 0 0
   7       0 loop0 10 0 80 10 0 0 0 0 0 20 10 0 0 0 0 0 0
`)
	writeFixture(t, netdev, netDevHeader+`    lo: 999999 1 0 0 0 0 0 0 999999 1 0 0 0 0 0 0
  eth0: 1000 10 0 0 0 0 0 0 2000 10 0 0 0 0 0 0
`)
	primeIO()
	if !ioPrev.ok {
		t.Fatal("first reading should prime the sampler")
	}
	ioPrev.at = ioPrev.at.Add(-time.Second)

	writeFixture(t, diskstats, `   8       0 sda 1100 0 22048 8000 500 0 8000 1000 0 10600 9000 0 0 0 0 0 0
   8       1 sda1 1100 0 22048 8000 500 0 8000 1000 0 10600 9000 0 0 0 0 0 0
   8      16 sdb 10 0 80 10 0 0 0 0 0 20 10 0 0 0 0 0 0
   9       0 md0 10 0 80 10 0 0 0 0 0 20 10 0 0 0 0 0 0
   7       0 loop0 10 0 80 10 0 0 0 0 0 20 10 0 0 0 0 0 0
`)
	writeFixture(t, netdev, netDevHeader+`    lo: 999999 1 0 0 0 0 0 0 999999 1 0 0 0 0 0 0
  eth0: 2000 10 0 0 0 0 0 0 4000 10 0 0 0 0 0 0
`)
	disks, net := sampleIO()
	if len(disks) != 2 {
		t.Fatalf("want sda and sdb only, got %+v", disks)
	}
	sda := disks[0]
	if sda.Device != "sda" || sda.Label != "QEMU HARDDISK drive-scsi1" || sda.ID != "t10.QEMU QEMU HARDDISK drive-scsi1" {
		t.Errorf("identity: %+v", sda)
	}
	if !sda.Rotational || disks[1].Rotational {
		t.Errorf("rotational flags: %v %v", sda.Rotational, disks[1].Rotational)
	}
	if sda.AwaitMs < 29.9 || sda.AwaitMs > 30.1 {
		t.Errorf("await: want 30 ms, got %.2f", sda.AwaitMs)
	}
	if sda.UtilPercent < 59 || sda.UtilPercent > 61 {
		t.Errorf("util: want ~60 %%, got %.1f", sda.UtilPercent)
	}
	if want := int64(2048 * 512); sda.ReadBps < want-2000 || sda.ReadBps > want+2000 {
		t.Errorf("read bps: want ~%d, got %d", want, sda.ReadBps)
	}
	if sda.WriteBps != 0 {
		t.Errorf("no writes happened, got %d B/s", sda.WriteBps)
	}
	if disks[1].AwaitMs != 0 || disks[1].UtilPercent != 0 {
		t.Errorf("idle sdb should read zero: %+v", disks[1])
	}
	if net.RxBps < 7900 || net.RxBps > 8100 || net.TxBps < 15900 || net.TxBps > 16100 {
		t.Errorf("net: want 8 kbps in / 16 kbps out (loopback excluded), got %+v", net)
	}
}

func TestSampleIO_FirstCallReportsNothing(t *testing.T) {
	diskstats, netdev, sysBlock := useFakeProc(t)
	fakeBlockDevice(t, sysBlock, "sda", 0, nil)
	writeFixture(t, diskstats, "   8       0 sda 1 0 8 1 0 0 0 0 0 1 1 0 0 0 0 0 0\n")
	writeFixture(t, netdev, netDevHeader)
	disks, net := sampleIO()
	if disks != nil || net != (NetIO{}) {
		t.Errorf("without a previous reading nothing can be averaged: %+v %+v", disks, net)
	}
	if !ioPrev.ok {
		t.Error("the call should still have taken the first reading")
	}
}

func TestSampleIO_UnreadableProcIsSilent(t *testing.T) {
	useFakeProc(t)
	primeIO()
	disks, net := sampleIO()
	if disks != nil || net != (NetIO{}) {
		t.Errorf("no /proc/diskstats: want nothing, got %+v %+v", disks, net)
	}
}

// The device letter is never the identity: it moved between boots on the
// production VM. The model alone cannot be, every disk of a VM shares it.
func TestDiskIdentity_PrefersStableIDs(t *testing.T) {
	_, _, sysBlock := useFakeProc(t)
	fakeBlockDevice(t, sysBlock, "sda", 0, map[string]string{"device/model": "Virtual Disk", "device/wwid": "naa.6002248006f996de68d2e5552efcdf0c"})
	fakeBlockDevice(t, sysBlock, "sdb", 0, map[string]string{"device/model": "ST12000VN0008", "device/serial": "ZV70ABCD"})
	fakeBlockDevice(t, sysBlock, "sdc", 0, map[string]string{"device/model": "Some Disk"})
	fakeBlockDevice(t, sysBlock, "sdd", 0, map[string]string{"wwid": "eui.0025385b01234567"})
	fakeBlockDevice(t, sysBlock, "sde", 0, nil)

	cases := map[string][2]string{
		"sda": {"naa.6002248006f996de68d2e5552efcdf0c", "Virtual Disk e5552efcdf0c"},
		"sdb": {"ZV70ABCD", "ST12000VN0008 ZV70ABCD"},
		"sdc": {"Some Disk", "Some Disk"},
		"sdd": {"eui.0025385b01234567", "385b01234567"},
		"sde": {"sde", "sde"},
	}
	for name, want := range cases {
		id, label := diskIdentity(name)
		if id != want[0] || label != want[1] {
			t.Errorf("%s: want %q / %q, got %q / %q", name, want[0], want[1], id, label)
		}
	}
}

// The thresholds come from the load test that broke on 2026-10-15: disks at
// 56-82 % busy, under the old fixed line, while every request waited 27-46 ms
// against 7-8 ms when the run held.
func TestDiskSaturated_WaitAgainstRestDecides(t *testing.T) {
	if got := SaturationAwaitMs(7.5); got != SaturatedAwaitFloorMs {
		t.Errorf("2.5 x 7.5 ms = 18.75, the 20 ms floor wins: got %.2f", got)
	}
	if got := SaturationAwaitMs(12); got != 30 {
		t.Errorf("12 ms at rest -> 30 ms line, got %.2f", got)
	}
	if got := SaturationAwaitMs(0); got != SaturatedAwaitFloorMs {
		t.Errorf("an unknown rest value takes the 4 ms floor: got %.2f", got)
	}
	if !DiskSaturated(35, 7.5, 60, 10) {
		t.Error("the 15-10 plateau that broke must read as saturated")
	}
	if DiskSaturated(8, 7.5, 60, 10) {
		t.Error("the plateau that held must not")
	}
	if DiskSaturated(35, 7.5, 5, 5) {
		t.Error("a high wait with a near idle disk and idle CPUs is one slow request, not saturation")
	}
	if !DiskSaturated(35, 7.5, 5, 40) {
		t.Error("iowait corroborates when the busy share does not")
	}
	if !IdleAwaitCandidate(6, 3) || IdleAwaitCandidate(6, 50) || IdleAwaitCandidate(0, 1) {
		t.Error("a rest candidate served requests while nearly idle")
	}
}
