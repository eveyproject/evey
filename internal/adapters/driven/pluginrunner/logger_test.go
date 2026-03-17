package pluginrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginLoggerCreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	logger := NewPluginLogger(dir)
	defer logger.Close()

	logger.WriteLog("plg-test-1", "INFO", "hello world", map[string]string{"key": "val"})

	logPath := filepath.Join(dir, "plg-test-1.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	var entry map[string]any
	err = json.Unmarshal(data[:len(data)-1], &entry) // strip trailing newline
	require.NoError(t, err)

	assert.Equal(t, "INFO", entry["level"])
	assert.Equal(t, "hello world", entry["msg"])
	assert.Equal(t, "plg-test-1", entry["plugin"])
	assert.Equal(t, "val", entry["key"])
	assert.NotEmpty(t, entry["ts"])
}

func TestPluginLoggerMultipleEntries(t *testing.T) {
	dir := t.TempDir()
	logger := NewPluginLogger(dir)
	defer logger.Close()

	logger.WriteLog("plg-2", "INFO", "first", nil)
	logger.WriteLog("plg-2", "ERROR", "second", nil)

	data, err := os.ReadFile(filepath.Join(dir, "plg-2.log"))
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 2)
}

func TestPluginLogWriterInterface(t *testing.T) {
	dir := t.TempDir()
	logger := NewPluginLogger(dir)
	defer logger.Close()

	w := logger.LogWriter("plg-3", "WARN")
	_, err := w.Write([]byte("some output\n"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "plg-3.log"))
	require.NoError(t, err)

	var entry map[string]any
	err = json.Unmarshal(data[:len(data)-1], &entry)
	require.NoError(t, err)
	assert.Equal(t, "WARN", entry["level"])
	assert.Equal(t, "some output", entry["msg"])
}
