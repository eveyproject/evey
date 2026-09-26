// The roots a library scan could not read, from the ScanLibrary response
// ({ result: ScanProgress }). Non-empty means the scan is partial: the
// reachable folders are up to date, these were skipped.
export function scanFailedPaths(data: unknown): string[] {
  const result = (data as { result?: { failedPaths?: unknown } } | null | undefined)?.result;
  const paths = result?.failedPaths;
  return Array.isArray(paths) ? paths.filter((p): p is string => typeof p === "string" && p !== "") : [];
}
