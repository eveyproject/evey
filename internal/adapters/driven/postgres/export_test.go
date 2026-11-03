package postgres

// PruneOldBackupsForTest exposes pruneOldBackups for the external
// migrate_backup_test.go suite - tests cover the prune logic directly
// without spinning a postgres container.
func PruneOldBackupsForTest(dir string, keep int) {
	pruneOldBackups(dir, keep)
}

// SetScanListBatchSizeForTest pages the scan-time lists n rows at a time and
// returns the restore. Not for parallel tests: the page size is package-wide.
func SetScanListBatchSizeForTest(n int) func() {
	old := scanListBatchSize
	scanListBatchSize = n
	return func() { scanListBatchSize = old }
}

// SetScanListBetweenBatchesForTest runs fn between two pages of a scan-time
// list and returns the restore. Not for parallel tests.
func SetScanListBetweenBatchesForTest(fn func()) func() {
	old := scanListBetweenBatches
	scanListBetweenBatches = fn
	return func() { scanListBetweenBatches = old }
}
