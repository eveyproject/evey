package maintenance

import (
	"testing"
	"time"
)

// carryAcksLocked's two refusals, exercised directly: an acknowledgement is
// about ONE diagnosis of ONE set of bytes. A status re-diagnosed under a newer
// detector version, or one whose verdict became repairable, starts afresh.
func TestCarryAcksLocked_RefusesADifferentDiagVersion(t *testing.T) {
	s := &SeekIndexService{scans: map[string]cachedScan{
		"lib": {res: ScanResult{Items: []SeekStatus{{Path: "/m/a.mkv", SizeBytes: 10, ModTimeUnix: 5, DiagVersion: 2, HasIndex: true, TrailingJunk: true, AckedAt: 1_700_000_000}}}, at: time.Now()},
	}}
	res := &ScanResult{Items: []SeekStatus{{Path: "/m/a.mkv", SizeBytes: 10, ModTimeUnix: 5, DiagVersion: 3, HasIndex: true, TrailingJunk: true}}}
	s.carryAcksLocked(res)
	if res.Items[0].Acked() {
		t.Fatal("an ack about a version-2 diagnosis must not follow the version-3 one")
	}
}

func TestCarryAcksLocked_RefusesAFileThatBecameRepairable(t *testing.T) {
	s := &SeekIndexService{scans: map[string]cachedScan{
		"lib": {res: ScanResult{Items: []SeekStatus{{Path: "/m/a.mkv", SizeBytes: 10, ModTimeUnix: 5, DiagVersion: SeekDiagVersion, HasIndex: true, TrailingJunk: true, AckedAt: 1_700_000_000}}}, at: time.Now()},
	}}
	// Same bytes on disk, but this scan's diagnosis lost the index: repairable,
	// which no acknowledgement may hide.
	res := &ScanResult{Items: []SeekStatus{{Path: "/m/a.mkv", SizeBytes: 10, ModTimeUnix: 5, DiagVersion: SeekDiagVersion, HasIndex: false}}}
	s.carryAcksLocked(res)
	if res.Items[0].Acked() {
		t.Fatal("a repairable verdict must not inherit an ack")
	}
}

func TestCarryAcksLocked_CarriesAndRebuildsRollups(t *testing.T) {
	s := &SeekIndexService{scans: map[string]cachedScan{
		"lib": {res: ScanResult{Items: []SeekStatus{{Path: "/m/a.mkv", SizeBytes: 10, ModTimeUnix: 5, DiagVersion: SeekDiagVersion, HasIndex: true, TrailingJunk: true, AckedAt: 1_700_000_000}}}, at: time.Now()},
	}}
	res := &ScanResult{Items: []SeekStatus{{Path: "/m/a.mkv", SizeBytes: 10, ModTimeUnix: 5, DiagVersion: SeekDiagVersion, HasIndex: true, TrailingJunk: true}}}
	res.Total = 1
	addToRollups(res, res.Items[0]) // what the scan loop had counted: informational, not acked
	if res.Informational != 1 || res.Acked != 0 {
		t.Fatalf("precondition: informational=%d acked=%d", res.Informational, res.Acked)
	}
	s.carryAcksLocked(res)
	if !res.Items[0].Acked() || res.Items[0].AckedAt != 1_700_000_000 {
		t.Fatalf("ack not carried: %+v", res.Items[0])
	}
	if res.Acked != 1 || res.Informational != 0 || res.Total != 1 {
		t.Fatalf("rollups not rebuilt after the carry: acked=%d informational=%d total=%d", res.Acked, res.Informational, res.Total)
	}
}
