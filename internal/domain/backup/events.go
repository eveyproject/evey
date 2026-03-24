package backup

import "time"

type BackupFailed struct {
	Error      string
	OccurredAt time.Time
}

func (e BackupFailed) EventName() string { return "backup.failed" }
