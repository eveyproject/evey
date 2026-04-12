import { useState, useCallback } from "react";

const KEY = "evey-recent-searches";
const MAX = 8;

function load(): string[] {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? "[]");
  } catch {
    return [];
  }
}

function save(items: string[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(items));
  } catch (err) { console.warn("localStorage write failed", err); }
}

export function useRecentSearches() {
  const [recents, setRecents] = useState<string[]>(load);

  const push = useCallback((query: string) => {
    const trimmed = query.trim();
    if (!trimmed) return;
    setRecents((prev) => {
      const next = [trimmed, ...prev.filter((q) => q !== trimmed)].slice(0, MAX);
      save(next);
      return next;
    });
  }, []);

  const remove = useCallback((query: string) => {
    setRecents((prev) => {
      const next = prev.filter((q) => q !== query);
      save(next);
      return next;
    });
  }, []);

  const clear = useCallback(() => {
    save([]);
    setRecents([]);
  }, []);

  return { recents, push, remove, clear };
}
