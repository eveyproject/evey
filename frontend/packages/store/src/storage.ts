import type { StateStorage } from "zustand/middleware";

// Default: use localStorage (web). Mobile can override with AsyncStorage.
let customStorage: StateStorage | null = null;

// Stores registered for late rehydration. React Native has no
// window.localStorage, and ESM evaluates the store modules BEFORE the app's
// setZustandStorage() call in _layout - so the storage captured at store
// creation was the no-op fallback, and "persisted" state (servers, theme,
// locale, auth mirrors) silently never persisted. The lazy proxy below fixes
// the WRITES; this registry fixes the READS: once the real storage lands,
// every persisted store re-reads itself from it.
type Rehydratable = { persist?: { rehydrate: () => Promise<void> | void } };
const persistedStores: Rehydratable[] = [];

export function registerPersistedStore(store: Rehydratable): void {
  persistedStores.push(store);
  // Late registration after the storage is already wired (unusual import
  // order): hydrate immediately rather than never.
  if (customStorage) void store.persist?.rehydrate();
}

export function setZustandStorage(storage: StateStorage) {
  customStorage = storage;
  for (const store of persistedStores) void store.persist?.rehydrate();
}

// One stable object whose methods resolve the backing storage AT CALL TIME.
// createJSONStorage(getStorage) invokes getStorage() immediately (zustand 5),
// so handing it getZustandStorage directly froze whatever existed at module
// evaluation - the no-op on React Native. Delegating per call means the
// storage wired later is the one actually used.
export const lazyZustandStorage: StateStorage = {
  getItem: (name) => getZustandStorage().getItem(name),
  setItem: (name, value) => getZustandStorage().setItem(name, value),
  removeItem: (name) => getZustandStorage().removeItem(name),
};

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
