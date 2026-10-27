import { useEffect, useState } from "react";

/**
 * False on the render that first shows `key`, true two frames later: what a
 * screen gates on it renders after the top of the page is on screen, not
 * before. A new key starts over.
 */
export function useAfterFirstPaint(key: string | undefined): boolean {
  const [readyKey, setReadyKey] = useState<string | undefined>(undefined);
  useEffect(() => {
    if (!key) return;
    let second = 0;
    const first = requestAnimationFrame(() => {
      second = requestAnimationFrame(() => setReadyKey(key));
    });
    return () => {
      cancelAnimationFrame(first);
      cancelAnimationFrame(second);
    };
  }, [key]);
  return key !== undefined && readyKey === key;
}
