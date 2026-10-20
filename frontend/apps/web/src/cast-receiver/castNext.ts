// castNext.ts - the receiver's next-episode logic, pure and testable.
//
// A Chromecast has no user input: when an episode ends, the viewer used to
// have to wake the phone to chain the next one (real complaint, 2026-08-07).
// The receiver now asks the server itself - POST /cast-next, authenticated by
// the cast stream token it already holds - and self-loads the returned
// episode. main.ts owns the timers and the CAF calls; everything decidable
// is here, injected and unit-tested like the rest of receiverLogic.

import { episodeLabel } from "@evey/api";

export interface CastNextInfo {
  nextMediaItemId: string;
  token: string;
  title: string;
  seasonNumber: number;
  episodeNumber: number;
  seriesTitle: string;
}

// buildCastNextUrl derives the cast-next endpoint from the playing master's
// URL. The FULL query rides along: the token authenticates, and the caps
// params let the server prewarm with the exact fingerprint this receiver
// will request. Null = not an evey stream URL (foreign content).
export function buildCastNextUrl(streamUrl: string): string | null {
  try {
    const u = new URL(streamUrl);
    const m = /^(.*\/api\/v1\/media\/[^/]+)\//.exec(u.pathname + "/");
    if (!m || !u.searchParams.get("token")) return null;
    return `${u.origin}${m[1]}/cast-next${u.search}`;
  } catch {
    return null;
  }
}

// parseCastNextResponse validates the server's payload. Null = no usable
// next episode (malformed body, missing id/token) - the caller must treat
// it exactly like a 204.
export function parseCastNextResponse(data: unknown): CastNextInfo | null {
  if (typeof data !== "object" || data === null) return null;
  const d = data as Record<string, unknown>;
  const id = typeof d.next_media_item_id === "string" ? d.next_media_item_id : "";
  const token = typeof d.token === "string" ? d.token : "";
  if (!id || !token) return null;
  return {
    nextMediaItemId: id,
    token,
    title: typeof d.title === "string" ? d.title : "",
    seasonNumber: typeof d.season_number === "number" ? d.season_number : 0,
    episodeNumber: typeof d.episode_number === "number" ? d.episode_number : 0,
    seriesTitle: typeof d.series_title === "string" ? d.series_title : "",
  };
}

// buildNextMasterUrl swaps the media id and token of the CURRENT master to
// address the next episode, normalized onto the HLS master path (the
// universal shape - copy rungs included - whatever the current load was).
// Receiver-negotiated leftovers are dropped: abr=floor was the PREVIOUS
// episode's starvation rescue, maxKbps its throughput fit - the LOAD
// interceptor re-derives both for the new episode.
export function buildNextMasterUrl(streamUrl: string, next: Pick<CastNextInfo, "nextMediaItemId" | "token">): string | null {
  try {
    const u = new URL(streamUrl);
    if (!/\/api\/v1\/media\/[^/]+\//.test(u.pathname + "/")) return null;
    u.pathname = u.pathname.replace(
      /\/api\/v1\/media\/[^/]+(\/.*)?$/,
      `/api/v1/media/${encodeURIComponent(next.nextMediaItemId)}/hls/master.m3u8`,
    );
    u.searchParams.set("token", next.token);
    u.searchParams.delete("abr");
    u.searchParams.delete("maxKbps");
    return u.toString();
  } catch {
    return null;
  }
}

// shouldFetchNext gates the one prefetch per episode: close enough to the end
// that the answer stays fresh, far enough that the server-side warm-up has
// time to land segments before the switch. Short items (< 3 min) never
// chain - trailers and menus have no "next episode".
export const FETCH_NEXT_REMAINING_SEC = 90;

export function shouldFetchNext(posSec: number, durationSec: number, alreadyFetched: boolean): boolean {
  if (alreadyFetched) return false;
  if (!isFinite(posSec) || !isFinite(durationSec)) return false;
  if (durationSec < 180 || posSec <= 0) return false;
  return durationSec - posSec <= FETCH_NEXT_REMAINING_SEC;
}

// upNextLabel renders the overlay line. The receiver has no user session -
// the device's own language (a household setting on Chromecast) picks the
// static word, the rest is data.
export function upNextLabel(info: CastNextInfo, lang: string): { heading: string; title: string } {
  const heading = lang.toLowerCase().startsWith("fr") ? "À suivre" : "Up next";
  return {
    heading,
    title: episodeLabel({
      seriesTitle: info.seriesTitle,
      seasonNumber: info.seasonNumber,
      episodeNumber: info.episodeNumber,
      title: info.title,
    }),
  };
}
