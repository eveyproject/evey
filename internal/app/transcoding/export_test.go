package transcoding

import "time"

func (m *HLSManager) CleanupStale(maxIdle time.Duration) {
	m.cleanupStale(maxIdle)
}

func (m *HLSManager) SetQueueTimeout(d time.Duration) {
	m.queueTimeout = d
}

func IsTranscodeComplete(outputDir string, numStreams int) bool {
	return isTranscodeComplete(outputDir, numStreams)
}

func (m *HLSManager) UserSessionsCount(userID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.userSessions[userID]
}

func (m *HLSManager) UserSessionsTracked(userID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.userSessions[userID]
	return ok
}

func WriteCachedDecision(outputDir, videoDecision, audioDecision string) {
	writeCachedDecision(outputDir, videoDecision, audioDecision)
}

func MatchesCachedDecision(outputDir, videoDecision, audioDecision string) bool {
	return matchesCachedDecision(outputDir, videoDecision, audioDecision)
}
