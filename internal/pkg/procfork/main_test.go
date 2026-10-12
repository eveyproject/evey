package procfork

import (
	"os"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/sysinfo"
)

// The fork tests check what Command builds, not whether the host has room:
// the real admission read the machine's free memory and refused a Production
// fork whenever the full suite loaded the box (TestCommand_ProbeAndNoNiceAndNoMemCap,
// 2026-09-28). An unreadable reading admits everything, by contract; the
// admission itself is tested on its own fakes in admission_test.go.
func TestMain(m *testing.M) {
	defaultAdmission = newAdmission(func() (int64, bool) { return 0, false }, sysinfo.MemoryBudgetBytes)
	os.Exit(m.Run())
}
