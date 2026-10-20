/**
 * The one label for an episode, everywhere a viewer or an admin has to know
 * which one it is without opening it: "Series · S01E03 · Title". Players,
 * history, admin sessions and logs each grew their own variant (series and
 * title without the code, code and title without the series, "E5" here and
 * "E05" there); they all format through this now.
 *
 * Every part is optional and a missing one is left out, never shown as a
 * placeholder: no season gives "E03", no episode number gives no code, and
 * an item that is not an episode (no series, no code) is its title alone.
 */
export interface EpisodeLabelParts {
  seriesTitle?: string | null;
  seasonNumber?: number | null;
  episodeNumber?: number | null;
  title?: string | null;
}

const pad = (n: number) => String(n).padStart(2, "0");

/** "S01E03", "E03" when the season is unknown, "S01" without an episode, else "". */
export function episodeCode(seasonNumber?: number | null, episodeNumber?: number | null): string {
  const s = seasonNumber && seasonNumber > 0 ? `S${pad(seasonNumber)}` : "";
  const e = episodeNumber && episodeNumber > 0 ? `E${pad(episodeNumber)}` : "";
  return s + e;
}

/** "Series · S01E03 · Title", leaving out whatever part is missing. */
export function episodeLabel(parts: EpisodeLabelParts): string {
  return [parts.seriesTitle?.trim(), episodeCode(parts.seasonNumber, parts.episodeNumber), parts.title?.trim()]
    .filter(Boolean)
    .join(" · ");
}
