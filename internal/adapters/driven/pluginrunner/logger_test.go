package pluginrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPluginLoggerDefaults(t *testing.T) {
	l := NewPluginLogger("", 0)
	assert.Equal(t, defaultLogDir, l.logDir)
	assert.Equal(t, 30, l.maxAgeDays)
}

func TestNewPluginLoggerCustom(t *testing.T) {
	l := NewPluginLogger("/tmp/test-logs", 7)
	assert.Equal(t, "/tmp/test-logs", l.logDir)
	assert.Equal(t, 7, l.maxAgeDays)
}

func TestWriteLogCreatesFile(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 30)
	defer l.Close()

	l.WriteLog("test-plugin", "hello world")

	logPath := filepath.Join(dir, "test-plugin.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "hello world")
}

func TestWriterCreatesFile(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 30)
	defer l.Close()

	w := l.Writer("plg-1")
	_, err := w.Write([]byte("test output\n"))
	require.NoError(t, err)

	logPath := filepath.Join(dir, "plg-1.log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "test output")
}

func TestClosePluginRemovesWriter(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 30)

	l.WriteLog("plg-close", "before")
	l.ClosePlugin("plg-close")

	l.mu.Lock()
	_, exists := l.writers["plg-close"]
	l.mu.Unlock()
	assert.False(t, exists)
}

func TestSetRetentionDays(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 30)
	defer l.Close()

	_ = l.Writer("plg-ret")
	l.SetRetentionDays(7)

	assert.Equal(t, 7, l.maxAgeDays)
	l.mu.Lock()
	assert.Equal(t, 7, l.writers["plg-ret"].MaxAge)
	l.mu.Unlock()
}

func TestSetRetentionDaysZeroDefaultsTo30(t *testing.T) {
	l := NewPluginLogger(t.TempDir(), 10)
	l.SetRetentionDays(0)
	assert.Equal(t, 30, l.maxAgeDays)
}

type fakeLister struct {
	ids []string
	err error
}

func (f *fakeLister) ListIDs(_ context.Context) ([]string, error) {
	return f.ids, f.err
}

func TestCleanOrphanLogs(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 30)
	defer l.Close()

	// Create log files for 3 plugins
	for _, id := range []string{"plg-a", "plg-b", "plg-orphan"} {
		err := os.WriteFile(filepath.Join(dir, id+".log"), []byte("data"), 0o644)
		require.NoError(t, err)
	}

	lister := &fakeLister{ids: []string{"plg-a", "plg-b"}}
	l.cleanOrphanLogs(context.Background(), lister)

	// plg-a and plg-b should still exist
	_, err := os.Stat(filepath.Join(dir, "plg-a.log"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "plg-b.log"))
	assert.NoError(t, err)

	// plg-orphan should be removed
	_, err = os.Stat(filepath.Join(dir, "plg-orphan.log"))
	assert.True(t, os.IsNotExist(err))
}
