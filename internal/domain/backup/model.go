package backup

import "time"

type BackupManifest struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Version    string    `json:"version"`
	Tables     []string  `json:"tables"`
	Size       int64     `json:"size"`
	DumpSHA256 string    `json:"dump_sha256,omitempty"`
}
