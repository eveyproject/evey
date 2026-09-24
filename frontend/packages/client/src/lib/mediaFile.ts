// hasMediaFile: a real, downloadable single media file (movie or episode) rather than a
// container (series/season). Must NOT gate on filePath alone - the absolute
// server path is redacted for non-admins, so a filePath-only check would hide
// the download button for every non-admin viewer. fileSize is emitted to
// every viewer regardless of role, so it is the signal that works for everyone.
export function hasMediaFile(item: { fileSize?: string; filePath?: string }): boolean {
  return Number(item.fileSize ?? 0) > 0 || !!item.filePath;
}
