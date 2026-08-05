import type { DownloadDisplay } from "./downloads";

/**
 * How a downloaded item names itself with no server around.
 *
 * An episode's own title names nothing on its own ("The Spider and the Fly"),
 * and offline there is no second request to resolve its ancestors - so the
 * series and the S/E numbers are captured at download time and rendered from
 * the index. Entries downloaded before that capture carry none of it and keep
 * showing their bare title, never a broken prefix.
 */
export interface DownloadLabel {
  /** Line 1: the series when known, the item's own title otherwise. */
  primary: string;
  /** Line 2: "S1E5 · Episode title", "S1E5", or nothing. */
  secondary?: string;
}

function episodeCode(season?: number, episode?: number): string | undefined {
  if (season != null && season > 0 && episode != null && episode > 0) return `S${season}E${episode}`;
  if (episode != null && episode > 0) return `E${episode}`;
  if (season != null && season > 0) return `S${season}`;
  return undefined;
}

export function downloadLabel(item: DownloadDisplay & { title: string }): DownloadLabel {
  const code = episodeCode(item.seasonNumber, item.episodeNumber);
  if (!item.seriesTitle) {
    // A film (or a pre-capture entry): the title is already the whole answer.
    return code ? { primary: item.title, secondary: code } : { primary: item.title };
  }
  const rest = [code, item.title].filter(Boolean).join(" · ");
  return { primary: item.seriesTitle, secondary: rest || undefined };
}
