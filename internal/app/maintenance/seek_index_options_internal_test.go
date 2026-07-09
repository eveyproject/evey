package maintenance

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/mkv"
)

// The operator's deep-verify choice must reach the reindexer port: with the
// in-place strategy there is no backup, so the full-read proof is the only
// thing standing between a bad patch and a silently broken file.
func TestReindexOptionsFor_DeepVerifyReachesTheReindexer(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		deepVerify bool
		wantMode   string
		wantBackup bool
	}{
		{mkv.ModeInPlace, true, mkv.ModeInPlace, false},
		{mkv.ModeInPlace, false, mkv.ModeInPlace, false},
		{"auto", true, mkv.ModeAuto, true},
		{"", true, mkv.ModeRewrite, true},
		{"", false, mkv.ModeRewrite, true},
	} {
		got := reindexOptionsFor(tc.mode, tc.deepVerify)
		if got.Mode != tc.wantMode {
			t.Errorf("mode %q: got %q, want %q", tc.mode, got.Mode, tc.wantMode)
		}
		if got.DeepVerify != tc.deepVerify {
			t.Errorf("mode %q: DeepVerify = %v, want %v", tc.mode, got.DeepVerify, tc.deepVerify)
		}
		if got.KeepBackup != tc.wantBackup {
			t.Errorf("mode %q: KeepBackup = %v, want %v", tc.mode, got.KeepBackup, tc.wantBackup)
		}
	}
}
