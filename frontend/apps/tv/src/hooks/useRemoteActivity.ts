import { useCallback, useEffect, useRef, useState } from "react";
import { useTVEventHandler } from "react-native";

/**
 * True while the remote is in use: from any key (or focus move) until
 * `quietMs` pass without one. Re-renders only on the two edges - React drops
 * a state set to the value it already holds.
 */
export function useRemoteActivity(quietMs: number): boolean {
  const [active, setActive] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useTVEventHandler(
    useCallback(() => {
      setActive(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setActive(false), quietMs);
    }, [quietMs]),
  );

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  return active;
}
