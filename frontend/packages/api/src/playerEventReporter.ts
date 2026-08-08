import { eveyFetch } from "./fetcher";

// playerEventReporter - the client half of the playback-error beacon.
//
// A decode error painting the screen in coloured macroblocks, an hls.js
// recovery loop, an endless stall: the server sees none of it (every segment
// request it answered was a 200). The 2026-07-15 incident left ZERO server-side
// trace and had to be diagnosed by inference. This reporter posts what the
// player saw, fire-and-forget, so the incident exists in the admin logs.
//
// It throttles itself hard: a broken stream can fire the same error dozens of
// times a second, and the server's own rate limit would silently eat the
// excess anyway. Identical consecutive events are deduped, and a per-session
// budget caps the total - the FIRST few events carry all the diagnostic value.

export interface PlayerEventPayload {
  /** Event family, [a-z0-9_]: hls_fatal, hls_media_error, video_element_error... */
  type: string;
  /** The player's own detail string (hls.js data.details, MediaError.message). */
  details?: string;
  fatal?: boolean;
  /** HTTP status for network errors, MediaError.code for element-level ones. */
  code?: number;
  variant?: number;
  segment?: number;
  posSec?: number;
}

const DEDUPE_WINDOW_MS = 5_000;
const MAX_EVENTS_PER_SESSION = 20;

export type PlayerEventReporter = (evt: PlayerEventPayload) => void;

/** noopReporter swallows events - for players without a media id (previews). */
export const noopReporter: PlayerEventReporter = () => {};

export function createPlayerEventReporter(mediaId: string): PlayerEventReporter {
  let sent = 0;
  let lastKey = "";
  let lastAt = 0;

  return (evt: PlayerEventPayload) => {
    if (!mediaId || sent >= MAX_EVENTS_PER_SESSION) return;
    const key = `${evt.type}|${evt.details ?? ""}|${evt.code ?? 0}`;
    const now = Date.now();
    if (key === lastKey && now - lastAt < DEDUPE_WINDOW_MS) return;
    lastKey = key;
    lastAt = now;
    sent++;
    eveyFetch<void>({
      url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
      method: "POST",
      data: {
        type: evt.type,
        details: evt.details ?? "",
        fatal: evt.fatal ?? false,
        code: evt.code ?? 0,
        variant: evt.variant ?? 0,
        segment: evt.segment ?? 0,
        posSec: evt.posSec ?? 0,
      },
    }).catch(() => {
      // Telemetry must never break playback. Rate-limited (429) or offline:
      // the event is simply lost, and that is fine.
    });
  };
}
