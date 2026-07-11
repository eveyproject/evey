import type { StateStorage } from "zustand/middleware";

// Default: use localStorage (web). Mobile can override with AsyncStorage.
let customStorage: StateStorage | null = null;

export function setZustandStorage(storage: StateStorage) {
  customStorage = storage;
}

export function getZustandStorage(): StateStorage {
  if (customStorage) return customStorage;
  // Fallback to localStorage if available (web)
  if (typeof window !== "undefined" && window.localStorage) {
    return {
      getItem: (name) => localStorage.getItem(name),
      setItem: (name, value) => localStorage.setItem(name, value),
      removeItem: (name) => localStorage.removeItem(name),
    };
  }
  // No storage available - return no-op (prevents warnings)
  return {
    getItem: () => null,
    setItem: () => {},
    removeItem: () => {},
  };
}
