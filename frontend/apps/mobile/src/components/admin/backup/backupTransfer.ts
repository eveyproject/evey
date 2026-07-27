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

// Builds the absolute, token-authenticated URL for GET /api/v1/admin/backups/{id}.
// The server accepts the access token as a query param as a fallback to the
// Authorization header (see extractBearerToken server-side) - the same scheme
// the web app's anchor-tag download already relies on, since native
// download/upload modules don't reliably forward custom headers on redirects.
export function buildBackupDownloadUrl(baseURL: string, backupId: string, accessToken: string | null): string {
  const base = baseURL.replace(/\/+$/, "");
  const url = `${base}/api/v1/admin/backups/${encodeURIComponent(backupId)}`;
  return accessToken ? `${url}?token=${encodeURIComponent(accessToken)}` : url;
}

export function buildBackupRestoreUrl(baseURL: string): string {
  return `${baseURL.replace(/\/+$/, "")}/api/v1/admin/backups/restore`;
}
