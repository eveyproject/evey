package postgres

// PruneOldBackupsForTest exposes pruneOldBackups for the external
// migrate_backup_test.go suite - tests cover the prune logic directly
// without spinning a postgres container.
func PruneOldBackupsForTest(dir string, keep int) {
	pruneOldBackups(dir, keep)
}
