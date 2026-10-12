package maintenance

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// The operator's deep-verify choice must reach the reindexer port: with the
// in-place strategy there is no backup, so the full-read proof is the only
// thing standing between a bad patch and a silently broken file. skip_backup
// exists for repairing a nearly-full volume (no persistent copy per repaired
// file) and MUST force deep-verify on as the replacement safety net.
func TestReindexOptionsFor_OperatorChoicesReachTheReindexer(t *testing.T) {
	for _, tc := range []struct {
		name           string
		run            ReindexRunOptions
		wantMode       string
		wantBackup     bool
		wantDeepVerify bool
	}{
		{"inplace", ReindexRunOptions{Mode: mkv.ModeInPlace, DeepVerify: true}, mkv.ModeInPlace, false, true},
		{"inplace no deep-verify", ReindexRunOptions{Mode: mkv.ModeInPlace}, mkv.ModeInPlace, false, false},
		{"auto keeps backup by default", ReindexRunOptions{Mode: "auto", DeepVerify: true}, mkv.ModeAuto, true, true},
		{"empty mode = rewrite with backup", ReindexRunOptions{DeepVerify: true}, mkv.ModeRewrite, true, true},
		{"rewrite without deep-verify", ReindexRunOptions{}, mkv.ModeRewrite, true, false},
		{"auto skip-backup forces deep-verify", ReindexRunOptions{Mode: "auto", SkipBackup: true}, mkv.ModeAuto, false, true},
		{"rewrite skip-backup forces deep-verify", ReindexRunOptions{SkipBackup: true}, mkv.ModeRewrite, false, true},
		{"inplace ignores skip-backup", ReindexRunOptions{Mode: mkv.ModeInPlace, SkipBackup: true}, mkv.ModeInPlace, false, true},
	} {
		got := reindexOptionsFor(tc.run)
		if got.Mode != tc.wantMode {
			t.Errorf("%s: mode = %q, want %q", tc.name, got.Mode, tc.wantMode)
		}
		if got.DeepVerify != tc.wantDeepVerify {
			t.Errorf("%s: DeepVerify = %v, want %v", tc.name, got.DeepVerify, tc.wantDeepVerify)
		}
		if got.KeepBackup != tc.wantBackup {
			t.Errorf("%s: KeepBackup = %v, want %v", tc.name, got.KeepBackup, tc.wantBackup)
		}
	}
}
