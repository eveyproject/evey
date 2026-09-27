import { useCallback, useEffect, useRef } from "react";
import { AppState } from "react-native";
import { useFocusEffect } from "expo-router";

// useRefetchOnReturn re-reads a query when its screen is shown AGAIN: back from
// a screen pushed on top of it (the player), or the app back from background
// while it is the visible one. A native stack keeps the screen mounted through
// both, so neither remounts it nor refetches anything on its own. The first
// focus is skipped: the mount already fetched.
export function useRefetchOnReturn(refetch: () => unknown): void {
  const refetchRef = useRef(refetch);
  refetchRef.current = refetch;
  const focusedRef = useRef(false);
  const shownOnceRef = useRef(false);

  useFocusEffect(
    useCallback(() => {
      focusedRef.current = true;
      if (shownOnceRef.current) void refetchRef.current();
      shownOnceRef.current = true;
      return () => {
        focusedRef.current = false;
      };
    }, []),
  );

  useEffect(() => {
    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active" && focusedRef.current) void refetchRef.current();
    });
    return () => sub.remove();
  }, []);
}
