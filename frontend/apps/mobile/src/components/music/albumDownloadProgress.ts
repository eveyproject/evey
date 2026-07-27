// Downloading a whole album reuses the existing per-track startDownload
// mechanism (there is no offline archive/unzip support on mobile - see the
// downloadAlbum report note), so its progress is the average of each
// individual track's own 0..1 progress. A track with unknown total size
// reports NaN progress (see downloads.ts); it's treated as 0 here rather than
// poisoning the whole average with NaN.
export function averageAlbumProgress(trackProgresses: number[]): number {
  if (trackProgresses.length === 0) return 0;
  const sum = trackProgresses.reduce((acc, p) => acc + (Number.isFinite(p) ? p : 0), 0);
  return sum / trackProgresses.length;
}
