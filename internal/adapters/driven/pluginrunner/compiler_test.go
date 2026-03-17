package pluginrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func esbuildAvailable() bool {
	_, err := exec.LookPath("esbuild")
	return err == nil
}

func TestCompileWidget(t *testing.T) {
	if !esbuildAvailable() {
		t.Skip("esbuild not in PATH")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "widget.jsx")
	out := filepath.Join(dir, "widget.bundle.js")

	require.NoError(t, os.WriteFile(src, []byte(`
export default function Widget() {
  return <div>Hello</div>;
}
`), 0o644))

	compiler := NewEsbuildCompiler()
	err := compiler.CompileWidget(src, out)
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Widget")
	assert.NotEmpty(t, data)
}

func TestCompileWidgetTSX(t *testing.T) {
	if !esbuildAvailable() {
		t.Skip("esbuild not in PATH")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "widget.tsx")
	out := filepath.Join(dir, "widget.bundle.js")

	require.NoError(t, os.WriteFile(src, []byte(`
interface Props { name: string }
export default function Widget({ name }: Props) {
  return <div>Hello {name}</div>;
}
`), 0o644))

	compiler := NewEsbuildCompiler()
	err := compiler.CompileWidget(src, out)
	require.NoError(t, err)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Widget")
}

func TestCompileWidgetInvalidSource(t *testing.T) {
	if !esbuildAvailable() {
		t.Skip("esbuild not in PATH")
	}

	compiler := NewEsbuildCompiler()
	err := compiler.CompileWidget("/nonexistent/widget.jsx", "/tmp/out.js")
	assert.Error(t, err)
}
