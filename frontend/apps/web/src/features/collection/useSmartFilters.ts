import { useState, useCallback } from "react";
import type { SmartFilter } from "./SmartCollectionDialog";

const KEY = "evey:smart-filters";

function load(): SmartFilter[] {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? "[]") as SmartFilter[];
  } catch {
    return [];
  }
}

function persist(filters: SmartFilter[]) {
  localStorage.setItem(KEY, JSON.stringify(filters));
}

export function useSmartFilters() {
  const [filters, setFilters] = useState<SmartFilter[]>(load);

  const save = useCallback((filter: SmartFilter) => {
    setFilters((prev) => {
      const exists = prev.findIndex((f) => f.id === filter.id);
      const next = exists >= 0
        ? prev.map((f) => (f.id === filter.id ? filter : f))
        : [...prev, filter];
      persist(next);
      return next;
    });
  }, []);

  const remove = useCallback((id: string) => {
    setFilters((prev) => {
      const next = prev.filter((f) => f.id !== id);
      persist(next);
      return next;
    });
  }, []);

  return { filters, save, remove };
}
