package transcoding

import "time"

func (m *HLSManager) CleanupStale(maxIdle time.Duration) {
	m.cleanupStale(maxIdle)
}

func IsTranscodeComplete(outputDir string, numStreams int) bool {
	return isTranscodeComplete(outputDir, numStreams)
}
