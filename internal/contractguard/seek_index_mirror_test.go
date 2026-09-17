package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
)

// The seek-index admin screens render a PROGNOSIS of what a repair will
// accept: the audio-delay defect floor and the space-preflight headroom shown
// to the operator are mirrors of server constants. A drifted mirror does not
// fail anywhere - the screen just quietly predicts something the repair then
// refuses (or accepts). This pins the shared client copy to the Go values.

const seekIndexHelpersTS = "frontend/packages/api/src/seekIndexHelpers.ts"

func tsConst(t *testing.T, src, name, expr string) string {
	t.Helper()
	re := regexp.MustCompile(`export const ` + name + ` = ` + expr + `;`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("%s: constant %q not found as /%s/ - if it was rewritten, update this guard so the pin keeps holding", seekIndexHelpersTS, name, expr)
	}
	return m[1]
}

func TestSeekIndexClientMirrorsMatchServer(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, seekIndexHelpersTS))
	if err != nil {
		t.Fatalf("read shared seek-index helpers: %v", err)
	}
	src := string(b)

	floor := tsConst(t, src, "AUDIO_DELAY_FLOOR_SEC", `([0-9.]+)`)
	got, err := strconv.ParseFloat(floor, 64)
	if err != nil {
		t.Fatalf("parse AUDIO_DELAY_FLOOR_SEC %q: %v", floor, err)
	}
	if got != appmaintenance.AudioDelayFlagSec {
		t.Errorf("AUDIO_DELAY_FLOOR_SEC = %v, server maintenance.AudioDelayFlagSec = %v", got, appmaintenance.AudioDelayFlagSec)
	}

	back := tsConst(t, src, "INTERLEAVE_BACKSTEP_FLOOR_SEC", `([0-9.]+)`)
	gotBack, err := strconv.ParseFloat(back, 64)
	if err != nil {
		t.Fatalf("parse INTERLEAVE_BACKSTEP_FLOOR_SEC %q: %v", back, err)
	}
	if gotBack != appmaintenance.InterleaveBackstepFlagSec {
		t.Errorf("INTERLEAVE_BACKSTEP_FLOOR_SEC = %v, server maintenance.InterleaveBackstepFlagSec = %v", gotBack, appmaintenance.InterleaveBackstepFlagSec)
	}

	// volumeHeadroomBytes is unexported server-side, so pin both files to the
	// same MiB figure read out of their source.
	mib := tsConst(t, src, "VOLUME_HEADROOM_BYTES", `(\d+) \* 1024 \* 1024`)

	gb, err := os.ReadFile(filepath.Join(repoRoot, "internal/app/maintenance/seek_index_volumes.go"))
	if err != nil {
		t.Fatalf("read seek_index_volumes.go: %v", err)
	}
	goRe := regexp.MustCompile(`const volumeHeadroomBytes = (\d+) << 20`)
	gm := goRe.FindStringSubmatch(string(gb))
	if gm == nil {
		t.Fatal("seek_index_volumes.go: volumeHeadroomBytes not found as /const volumeHeadroomBytes = N << 20/ - update this guard alongside the constant")
	}
	if mib != gm[1] {
		t.Errorf("VOLUME_HEADROOM_BYTES = %s MiB, server volumeHeadroomBytes = %s MiB", mib, gm[1])
	}
}
