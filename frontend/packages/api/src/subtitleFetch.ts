/**
 * Fetching one embedded subtitle track from /api/v1/media/{id}/subtitle/{n}.
 *
 * The server answers 200 with the bytes (WebVTT or a decoded-image blob) when
 * it has them, 202 with Retry-After while it is still producing them - a
 * track without a block index is a read of the whole film, minutes on a large
 * remux - and 422 when this build cannot decode the track at all. Before the
 * 202 existed the request simply hung: behind a proxy with a read timeout the
 * viewer got a 504, and every client showed nothing and never asked again.
 *
 * One implementation for the four clients, so they all poll the same way and
 * all have a "preparing" state to show.
 */

export type SubtitleFetchState = "preparing" | "ready" | "failed" | "cancelled";

export interface SubtitleFetchResult {
  state: SubtitleFetchState;
  /** The bytes, when state is "ready". */
  bytes: Uint8Array | null;
  /** The HTTP status that ended the fetch (0 when cancelled or on a network error). */
  status: number;
}

export interface SubtitleFetchOptions {
  /** Performs one request; the caller decides how the token travels. */
  doFetch: () => Promise<Response>;
  /** Called on every 202, with the delay the server asked for. */
  onPreparing?: (retryAfterSec: number) => void;
  /** Resolves "cancelled" as soon as it aborts; the server keeps producing. */
  signal?: AbortSignal;
  /** Overridable for tests. */
  sleep?: (ms: number, signal?: AbortSignal) => Promise<void>;
  /** Hard stop on polling, so a server that never finishes cannot hold a client forever. */
  maxAttempts?: number;
}

// Bounds on the delay the server suggests: a 1 s poll would hammer a server
// walking a film, and a client should not wait longer than this to notice the
// track is ready.
const MIN_RETRY_SEC = 2;
const MAX_RETRY_SEC = 30;
const DEFAULT_MAX_ATTEMPTS = 120; // 120 x 30 s = an hour, past any extraction deadline

/** The delay a 202 asks for, from the header or the body, bounded. */
export function retryAfterSeconds(res: Response, body?: unknown): number {
  let sec = Number(res.headers.get("Retry-After"));
  if (!Number.isFinite(sec) || sec <= 0) {
    const fromBody = body && typeof body === "object" ? (body as { retryAfterSec?: unknown }).retryAfterSec : undefined;
    sec = typeof fromBody === "number" ? fromBody : NaN;
  }
  if (!Number.isFinite(sec) || sec <= 0) sec = MIN_RETRY_SEC;
  return Math.min(MAX_RETRY_SEC, Math.max(MIN_RETRY_SEC, Math.round(sec)));
}

function defaultSleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) return resolve();
    const id = setTimeout(done, ms);
    function done() {
      signal?.removeEventListener("abort", done);
      clearTimeout(id);
      resolve();
    }
    signal?.addEventListener("abort", done, { once: true });
  });
}

/**
 * Fetches the track, following 202 answers until the server has it. Never
 * throws: a network error is a "failed" result like a 4xx/5xx, so a subtitle
 * can never take the playback down with it.
 */
export async function fetchSubtitleTrack(opts: SubtitleFetchOptions): Promise<SubtitleFetchResult> {
  const sleep = opts.sleep ?? defaultSleep;
  const max = opts.maxAttempts ?? DEFAULT_MAX_ATTEMPTS;
  for (let attempt = 0; attempt < max; attempt++) {
    if (opts.signal?.aborted) return { state: "cancelled", bytes: null, status: 0 };
    let res: Response;
    try {
      res = await opts.doFetch();
    } catch {
      return { state: "failed", bytes: null, status: 0 };
    }
    if (opts.signal?.aborted) return { state: "cancelled", bytes: null, status: 0 };
    if (res.status === 202) {
      let body: unknown;
      try {
        body = await res.json();
      } catch {
        body = undefined;
      }
      const sec = retryAfterSeconds(res, body);
      opts.onPreparing?.(sec);
      await sleep(sec * 1000, opts.signal);
      continue;
    }
    if (!res.ok) return { state: "failed", bytes: null, status: res.status };
    try {
      return { state: "ready", bytes: new Uint8Array(await res.arrayBuffer()), status: res.status };
    } catch {
      return { state: "failed", bytes: null, status: res.status };
    }
  }
  return { state: "failed", bytes: null, status: 202 };
}
