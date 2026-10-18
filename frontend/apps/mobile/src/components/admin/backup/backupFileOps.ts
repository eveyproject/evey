import { Directory, File, Paths } from "expo-file-system";
import { uploadAsync, FileSystemUploadType } from "expo-file-system/legacy";
import { getBaseURL } from "@evey/api";
import { refusalMessage } from "../plugins/pluginFileInstall";
import { buildBackupDownloadUrl, buildBackupRestoreUrl, isBackupRestorable, type BackupSummary } from "./backupTransfer";

/**
 * Downloads a backup archive into the app's document directory. Kept separate
 * from src/lib/downloads.ts (media downloads index): that mechanism is keyed
 * by mediaItemId, mints a per-media stream token, and persists into a
 * downloads list meant for movies/tracks - none of which fits an admin backup
 * archive, so this does not reuse it (see report for the alternative it would
 * need if we ever want the two to share one code path).
 */
export async function downloadBackupFile(backup: BackupSummary, accessToken: string | null): Promise<string> {
  if (!isBackupRestorable(backup)) throw new Error("invalid backup id");
  const url = buildBackupDownloadUrl(getBaseURL(), backup.id);
  const dir = new Directory(Paths.document, "backups");
  if (!dir.exists) dir.create({ intermediates: true, idempotent: true });
  const destination = new File(dir, `${backup.id}.tar.gz`);
  const file = await File.downloadFileAsync(url, destination, {
    headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined,
    idempotent: true,
  });
  return file.uri;
}

/**
 * Restores from an EXTERNAL archive the admin picked off the device (mirror of
 * the web RestoreUploadModal). Streams the file straight to the multipart
 * restore endpoint - no download round-trip, since the file is already local.
 */
export async function restoreFromFile(fileUri: string, accessToken: string | null): Promise<void> {
  const result = await uploadAsync(buildBackupRestoreUrl(getBaseURL()), fileUri, {
    httpMethod: "POST",
    uploadType: FileSystemUploadType.MULTIPART,
    fieldName: "backup",
    mimeType: "application/gzip",
    headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined,
  });
  if (result.status < 200 || result.status >= 300) {
    throw new Error(refusalMessage(result.body, "restore failed"));
  }
}

/**
 * Restores a backup that already exists server-side. There is no
 * restore-by-id endpoint (see backup_handler.go): the server only exposes a
 * raw download and a multipart-upload restore, so even a backup already on
 * disk must round-trip through the device. Downloading to a temp file and
 * streaming it back via expo-file-system's multipart upload (instead of
 * holding the whole archive as an in-memory Blob) keeps a large database dump
 * from blowing the JS heap.
 */
export async function restoreExistingBackup(backup: BackupSummary, accessToken: string | null): Promise<void> {
  if (!isBackupRestorable(backup)) throw new Error("invalid backup id");
  const baseURL = getBaseURL();
  const downloadUrl = buildBackupDownloadUrl(baseURL, backup.id);
  const tempDir = new Directory(Paths.cache, "backup-restore");
  if (!tempDir.exists) tempDir.create({ intermediates: true, idempotent: true });
  const tempFile = new File(tempDir, `${backup.id}.tar.gz`);
  await File.downloadFileAsync(downloadUrl, tempFile, {
    headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined,
    idempotent: true,
  });
  try {
    const result = await uploadAsync(buildBackupRestoreUrl(baseURL), tempFile.uri, {
      httpMethod: "POST",
      uploadType: FileSystemUploadType.MULTIPART,
      fieldName: "backup",
      mimeType: "application/gzip",
      headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined,
    });
    if (result.status < 200 || result.status >= 300) {
      throw new Error(refusalMessage(result.body, "restore failed"));
    }
  } finally {
    // Best-effort cleanup of the temp copy - a leftover few-MB file in the
    // cache dir is not worth failing the (already successful) restore over.
    try { tempFile.delete(); } catch { /* ignore */ }
  }
}
