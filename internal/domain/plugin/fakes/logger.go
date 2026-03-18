package fakes

import "sync"

type LogEntry struct {
	PluginID string
	Line     string
}

type FakePluginLogger struct {
	mu      sync.Mutex
	Entries []LogEntry
}

func NewFakePluginLogger() *FakePluginLogger {
	return &FakePluginLogger{}
}

func (l *FakePluginLogger) WriteLog(pluginID, line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Entries = append(l.Entries, LogEntry{PluginID: pluginID, Line: line})
}
