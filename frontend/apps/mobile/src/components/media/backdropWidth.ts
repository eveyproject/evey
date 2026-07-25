// The artwork endpoint snaps ?w= to its own fixed variant table server-side
// (see internal/adapters/driving/http/artwork_handler.go's allowedWidths) -
// mirroring that table here would just drift the day the server adds a size.
// The client's only job is to say how many physical pixels the backdrop
// occupies, the same thing the web hero does with `sizes="100vw"` plus a
// DPR-aware srcSet: request the full viewport width at its native density.
export function backdropWidthForViewport(viewportWidthPx: number, pixelRatio: number): number {
  const px = Math.round(viewportWidthPx * pixelRatio);
  return Number.isFinite(px) && px > 0 ? px : 0;
}
