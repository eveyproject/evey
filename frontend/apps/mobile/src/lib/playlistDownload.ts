import type { V1MediaItem } from "@evey/api";
import type { StartDownloadOptions } from "./downloads";

// Downloading a playlist is downloading its titles one by one, each with what
// the offline player needs (container, audio codecs for the decoder check,
// rating for the offline parental gate). A playlist row carries none of that,
// so every title is read first - the phone keeps one file per title, it gets
// no ZIP (a ZIP is not playable offline).

export interface PlaylistDownloadDeps {
  getItem: (id: string) => Promise<V1MediaItem>;
  isDownloaded: (id: string) => Promise<boolean>;
  start: (id: string, title: string, opts: StartDownloadOptions) => Promise<unknown>;
  authContext: (item: V1MediaItem) => Partial<StartDownloadOptions>;
}

export interface PlaylistDownloadResult {
  started: number;
  skipped: number;
  failed: number;
}

const DOWNLOADABLE = new Set(["MEDIA_ITEM_TYPE_MOVIE", "MEDIA_ITEM_TYPE_EPISODE"]);

export function audioCodecsOf(item?: { streams?: { type?: string; codec?: string }[] }): string[] | undefined {
  const codecs = (item?.streams ?? [])
    .filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.codec)
    .map((s) => s.codec as string);
  return codecs.length > 0 ? codecs : undefined;
}

// downloadPlaylistItems downloads, in order, every movie or episode not
// already on the device. One title failing does not stop the others.
export async function downloadPlaylistItems(ids: string[], deps: PlaylistDownloadDeps): Promise<PlaylistDownloadResult> {
  const result: PlaylistDownloadResult = { started: 0, skipped: 0, failed: 0 };
  for (const id of ids) {
    try {
      if (await deps.isDownloaded(id)) {
        result.skipped++;
        continue;
      }
      const item = await deps.getItem(id);
      if (!DOWNLOADABLE.has(item.type ?? "")) {
        result.skipped++;
        continue;
      }
      await deps.start(id, item.title ?? id, {
        kind: "video",
        formatHint: item.containerFormat,
        audioCodecs: audioCodecsOf(item),
        display: { seriesTitle: item.seriesTitle || undefined, episodeNumber: item.indexNumber },
        ...deps.authContext(item),
      });
      result.started++;
    } catch {
      result.failed++;
    }
  }
  return result;
}
