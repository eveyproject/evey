import { useState, useCallback } from "react";

// TODO: replace localStorage with server persistence when backend supports 1-5 star ratings
const KEY_PREFIX = "evey:star-rating:";

function load(mediaId: string): number {
  try {
    const v = localStorage.getItem(KEY_PREFIX + mediaId);
    if (v === null) return 0;
    const n = parseInt(v, 10);
    return n >= 1 && n <= 5 ? n : 0;
  } catch {
    return 0;
  }
}

function save(mediaId: string, value: number) {
  try {
    if (value === 0) localStorage.removeItem(KEY_PREFIX + mediaId);
    else localStorage.setItem(KEY_PREFIX + mediaId, String(value));
  } catch {}
}

export function useStarRating(mediaId: string | undefined) {
  const [rating, setRatingState] = useState<number>(() => (mediaId ? load(mediaId) : 0));

  const setRating = useCallback(
    (value: number) => {
      if (!mediaId) return;
      // clicking current rating removes it
      const next = rating === value ? 0 : value;
      save(mediaId, next);
      setRatingState(next);
    },
    [mediaId, rating],
  );

  return { rating, setRating };
}
