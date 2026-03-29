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
