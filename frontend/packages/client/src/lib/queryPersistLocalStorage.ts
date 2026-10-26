import type { PersistStorage } from "./queryPersist";

// The browsers' query cache (web, the browser television): localStorage,
// whose quota (about 5 MB) the cache stays well under. Private modes that
// refuse it read as a cache miss.
const KEY = "evey-query-cache";

export const localQueryStorage: PersistStorage = {
  async read() {
    try {
      return globalThis.localStorage?.getItem(KEY) ?? null;
    } catch {
      return null;
    }
  },
  async write(value) {
    try {
      globalThis.localStorage?.setItem(KEY, value);
    } catch {
      /* quota or private mode: the next start fetches */
    }
  },
  async remove() {
    try {
      globalThis.localStorage?.removeItem(KEY);
    } catch {
      /* nothing stored */
    }
  },
};
