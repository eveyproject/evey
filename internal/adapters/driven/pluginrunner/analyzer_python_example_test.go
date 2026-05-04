package pluginrunner

import (
	"testing"
)

// Anti-regression: examples/plugins/python-example contains the comment
// "# pip install grpcio grpcio-tools" which previously triggered a false-positive
// runtime_install Critical finding. The strip-line-comments normalization
// step must suppress it.
func TestPythonExample_NoFalsePositiveOnPipInstallComment(t *testing.T) {
	rep, err := AnalyzePlugin("../../../../examples/plugins/python-example")
	if err != nil {
		t.Fatalf("AnalyzePlugin: %v", err)
	}
	for _, f := range rep.Findings {
		if f.Category == "runtime_install" {
			t.Errorf("unexpected runtime_install finding in python-example: %s:%d %q", f.File, f.Line, f.Pattern)
		}
	}
}
