package pluginrunner

import (
	"fmt"
	"os/exec"
)

type EsbuildCompiler struct{}

func NewEsbuildCompiler() *EsbuildCompiler {
	return &EsbuildCompiler{}
}

func (c *EsbuildCompiler) CompileWidget(srcPath, outPath string) error {
	cmd := exec.Command("esbuild", srcPath,
		"--bundle",
		"--format=esm",
		"--jsx=automatic",
		"--external:react",
		"--external:react-dom",
		"--outfile="+outPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("esbuild compile: %s: %w", string(out), err)
	}
	return nil
}
