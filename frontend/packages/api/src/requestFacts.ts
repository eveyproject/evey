// The two facts a request shows beside its title, formatted once for the
// three apps: the release date the provider gave and the community rating.

/**
 * A provider date "YYYY-MM-DD" in the viewer's locale, without a time of day.
 * Built from its parts: `new Date("2024-05-12")` is midnight UTC, which a
 * viewer west of Greenwich would read back as the 11th.
 */
export function formatReleaseDate(iso: string | undefined, locale: string | undefined): string | undefined {
  if (!iso) return undefined;
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso.trim());
  if (!m) return undefined;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  if (Number.isNaN(d.getTime())) return undefined;
  try {
    return d.toLocaleDateString(locale || undefined, { year: "numeric", month: "short", day: "numeric" });
  } catch {
    return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
  }
}

/** "7.8" for a 0-10 score; undefined for unrated (0, absent or out of range). */
export function formatRating(rating: number | undefined): string | undefined {
  if (rating == null || !Number.isFinite(rating) || rating <= 0 || rating > 10) return undefined;
  return rating.toFixed(1);
}
