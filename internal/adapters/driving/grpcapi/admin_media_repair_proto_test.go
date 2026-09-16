package grpcapi

import (
	"testing"
	"time"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
)

// The repair journal entry crosses the wire field by field, every value
// distinct so a swap shows, and Reversible is the record's own verdict.
func TestRepairEntryToProto_MapsEveryField(t *testing.T) {
	started := time.Date(2026, 9, 16, 2, 0, 0, 0, time.UTC)
	rec := appmaintenance.RepairRecord{
		MediaID: "mi-1", Path: "/media/a.mkv", Op: appmaintenance.RepairAudioRetime, Job: "job-7",
		Started: started, Outcome: "applied", Error: "",
		SizeBefore: 1001, SizeAfter: 1002,
		ShiftSec:      map[uint64]float64{1: 12.0},
		AudibleBefore: 3, AudibleAfter: 3,
		Evidence: appmaintenance.RepairEvidence{Method: "track-coherence", LagSec: 12.0, Confidence: 0.87},
	}
	got := repairEntryToProto(rec)
	if got.AtMs != started.UnixMilli() || got.Op != string(appmaintenance.RepairAudioRetime) || got.Outcome != "applied" ||
		got.Job != "job-7" || got.MediaId != "mi-1" || got.Path != "/media/a.mkv" {
		t.Errorf("identity fields: %+v", got)
	}
	if got.EvidenceMethod != "track-coherence" || got.EvidenceLagSec != 12.0 || got.EvidenceConfidence != 0.87 {
		t.Errorf("evidence: %+v", got)
	}
	if got.SizeBefore != 1001 || got.SizeAfter != 1002 || got.AudibleBefore != 3 || got.AudibleAfter != 3 {
		t.Errorf("sizes/audible: %+v", got)
	}
	if got.ShiftSec[1] != 12.0 {
		t.Errorf("shift: %v", got.ShiftSec)
	}
	if !got.Reversible {
		t.Error("an applied retime with a recorded shift is reversible")
	}

	// A failed entry, or one without a recorded shift, has no way back.
	rec.Outcome = "failed"
	rec.Error = "guard: sound lost"
	got = repairEntryToProto(rec)
	if got.Reversible || got.Error != "guard: sound lost" {
		t.Errorf("failed entry: %+v", got)
	}
	rec.Outcome = "applied"
	rec.ShiftSec = nil
	if repairEntryToProto(rec).Reversible {
		t.Error("a retime without a recorded shift is not reversible")
	}
}
