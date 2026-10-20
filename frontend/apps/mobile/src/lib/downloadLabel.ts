import { episodeCode } from "@evey/api";
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
  /** Line 2: "S01E05 · Episode title", "S01E05", or nothing. */
  secondary?: string;
}

export function downloadLabel(item: DownloadDisplay & { title: string }): DownloadLabel {
  const code = episodeCode(item.seasonNumber, item.episodeNumber) || undefined;
  if (!item.seriesTitle) {
    // A film (or a pre-capture entry): the title is already the whole answer.
    return code ? { primary: item.title, secondary: code } : { primary: item.title };
  }
  const rest = [code, item.title].filter(Boolean).join(" · ");
  return { primary: item.seriesTitle, secondary: rest || undefined };
}
