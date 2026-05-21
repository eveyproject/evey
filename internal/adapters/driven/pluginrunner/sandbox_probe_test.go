package pluginrunner

import "testing"

func TestProbeSandbox_BasicModeIsNoop(t *testing.T) {
	// basic mode never wraps with bwrap, so there is nothing to probe and the
	// result must be nil even on a host without bwrap.
	if err := ProbeSandbox(sandboxModeBasic); err != nil {
		t.Fatalf("basic mode probe should be a no-op, got %v", err)
	}
}
