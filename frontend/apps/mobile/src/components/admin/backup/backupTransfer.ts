// Pure helpers for the admin backup download/restore flow. Deliberately free
// of any expo-file-system / native import so these stay unit-testable without
// mocking a native module (see backupFileOps.ts for the impure I/O side).

export interface BackupSummary {
  id: string;
  size?: number;
}

// A backup can only be downloaded/restored if the server actually returned an
// id to address it by - guards against a malformed manifest entry (e.g. a
// create-backup response that failed mid-write) surfacing an action that
// would 404 or, worse, restore an empty archive over the live database.
export function isBackupRestorable(backup: BackupSummary): boolean {
  return typeof backup.id === "string" && backup.id.length > 0;
}

// Builds the absolute URL for GET /api/v1/admin/backups/{id}. The access token
// rides the Authorization header of the native download, never the URL: the
// server refuses an account token in an admin URL (it would sit in proxy logs).
export function buildBackupDownloadUrl(baseURL: string, backupId: string): string {
  const base = baseURL.replace(/\/+$/, "");
  return `${base}/api/v1/admin/backups/${encodeURIComponent(backupId)}`;
}

export function buildBackupRestoreUrl(baseURL: string): string {
  return `${baseURL.replace(/\/+$/, "")}/api/v1/admin/backups/restore`;
}
