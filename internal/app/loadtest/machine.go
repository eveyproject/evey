package loadtest

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// machine.go - the whole machine, next to the container.
//
// The cgroup says what the CONTAINER burned. It cannot say what the box burned
// serving the same viewers outside it: a FUSE pool (mergerfs) reads the disks
// in its own process, a VM host or a NAS controller pays its own share, and a
// core waiting on the disk (iowait) is the one signal a saturated storage path
// leaves on the CPU gauge. /proc/stat is host-wide even inside a container, so
// the run reads it beside the cgroup and reports both: the gap between the
// two is what runs outside the container, and iowait is the storage talking.

// machineTicks is one reading of /proc/stat's aggregate cpu line, in USER_HZ
// ticks: busy is every state but idle and iowait, total is every state.
type machineTicks struct {
	busy, iowait, total uint64
	ok                  bool
}

// readMachineTicks parses fsRoot/proc/stat. Steal and guest follow the same
// convention as sysinfo.CPUBusyPercent: guest time is already inside user, and
// a core in iowait is not working, so iowait is counted apart and never as busy.
func readMachineTicks(fsRoot string) machineTicks {
	f, err := os.Open(filepath.Join(fsRoot, "proc/stat"))
	if err != nil {
		return machineTicks{}
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			return machineTicks{}
		}
		var t machineTicks
		// user nice system idle iowait irq softirq steal
		for i := 1; i < len(fields) && i <= 8; i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			t.total += v
			switch i {
			case 4: // idle
			case 5:
				t.iowait += v
			default:
				t.busy += v
			}
		}
		t.ok = true
		return t
	}
	return machineTicks{}
}

// machineDeltas turns two readings into the share of the interval the machine
// spent busy and the share it spent waiting on I/O (0..100 each).
func machineDeltas(prev, cur machineTicks) (busyPct, iowaitPct float64) {
	if !prev.ok || !cur.ok || cur.total <= prev.total {
		return 0, 0
	}
	total := float64(cur.total - prev.total)
	if cur.busy >= prev.busy {
		busyPct = float64(cur.busy-prev.busy) / total * 100
	}
	if cur.iowait >= prev.iowait {
		iowaitPct = float64(cur.iowait-prev.iowait) / total * 100
	}
	return min(busyPct, 100), min(iowaitPct, 100)
}

// readProcessRSS is the resident set of THIS process - the server itself,
// apart from whatever else the container holds (the image runs
// Meilisearch and Postgres beside it, and the cgroup counts them all). Read
// from /proc/self/status, which stays readable when the process is
// non-dumpable (/proc/self/io is not).
func readProcessRSS(fsRoot string) int64 {
	f, err := os.Open(filepath.Join(fsRoot, "proc/self/status"))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		return parseMeminfoKB(line)
	}
	return 0
}
