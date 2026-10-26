import { useCallback, useEffect, useRef, useState } from "react";

// A poster that failed to load was shown as a glyph for as long as the card
// lived: on a television the server answered a row's posters with empty
// bodies for a moment (2026-10-26, logcat: "offset (0) bigger than file size
// (0)"), the image cache then held those empty files, and the row stayed
// blank. Each failure is retried after a pause, on a URL the cache has not
// seen (`r=<attempt>`), and the attempt that worked is remembered for the
// rest of the session, so the next mount of that poster starts from it.

export const IMAGE_RETRY_DELAYS_MS = [1_500, 5_000, 15_000];

// Attempts known to work per URL, kept while the app lives.
const workingAttempt = new Map<string, number>();

export function retrySrc(uri: string, attempt: number): string {
  if (attempt === 0) return uri;
  return `${uri}${uri.includes("?") ? "&" : "?"}r=${attempt}`;
}

export interface ImageRetry {
  /** The URL to load now; undefined when there is nothing loadable yet. */
  src: string | undefined;
  /** Every retry failed: show the fallback. */
  failed: boolean;
  onError: () => void;
  onLoad: () => void;
}

/**
 * `uri` is the image's URL; `revive` is anything whose change should give a
 * failed image another go (the viewer's token: a 401 during a refresh is not
 * a missing poster).
 */
export function useImageRetry(uri: string | undefined, revive?: unknown): ImageRetry {
  const [attempt, setAttempt] = useState(() => (uri ? (workingAttempt.get(uri) ?? 0) : 0));
  const [failed, setFailed] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastRevive = useRef(revive);

  useEffect(() => {
    setAttempt(uri ? (workingAttempt.get(uri) ?? 0) : 0);
    setFailed(false);
  }, [uri]);

  useEffect(() => {
    if (lastRevive.current === revive) return;
    lastRevive.current = revive;
    if (failed) {
      setFailed(false);
      setAttempt(0);
    }
  }, [revive, failed]);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const onError = useCallback(() => {
    if (timer.current) return;
    const delay = IMAGE_RETRY_DELAYS_MS[attempt];
    if (delay === undefined) {
      setFailed(true);
      return;
    }
    timer.current = setTimeout(() => {
      timer.current = null;
      setAttempt(attempt + 1);
    }, delay);
  }, [attempt]);

  const onLoad = useCallback(() => {
    if (uri && attempt > 0) workingAttempt.set(uri, attempt);
  }, [uri, attempt]);

  return { src: uri && !failed ? retrySrc(uri, attempt) : undefined, failed, onError, onLoad };
}

/** Test seam. */
export function resetImageRetryForTests(): void {
  workingAttempt.clear();
}
