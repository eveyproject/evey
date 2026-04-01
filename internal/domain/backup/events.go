package backup

import "time"

type BackupFailed struct {
	Error      string
	OccurredAt time.Time
}

func (e BackupFailed) EventName() string { return "backup.failed" }

type BackupCreated struct {
	ID         string
	OccurredAt time.Time
}

func (e BackupCreated) EventName() string { return "backup.created" }

type BackupRestored struct {
	ID         string
	OccurredAt time.Time
}

func (e BackupRestored) EventName() string { return "backup.restored" }
