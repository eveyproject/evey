package backup

import "time"

// BackupFailed is published when a backup, manual or scheduled, fails.
type BackupFailed struct {
	Error      string
	OccurredAt time.Time
}

func (e BackupFailed) EventName() string { return "backup.failed" }

// BackupCreated is published once a backup is stored, manual or scheduled, so
// a webhook can confirm from outside that last night's backup really ran.
type BackupCreated struct {
	BackupID   string
	SizeBytes  int64
	OccurredAt time.Time
}

func (e BackupCreated) EventName() string { return "backup.created" }
