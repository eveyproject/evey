// From "/legacy": the root's file helpers throw at runtime in
// expo-file-system 55 (see apps/mobile downloads.ts).
import * as FileSystem from "expo-file-system/legacy";
import type { PersistStorage } from "../lib/queryPersist";

// The phone's and the television's query cache, one file in the cache
// directory (the OS may clear it; a missing file is a cache miss).
const FILE = ((FileSystem as unknown as { cacheDirectory?: string }).cacheDirectory ?? "") + "evey-query-cache.json";

export const fileQueryStorage: PersistStorage = {
  async read() {
    if (!FILE.startsWith("file:")) return null;
    const info = await FileSystem.getInfoAsync(FILE);
    return info.exists ? FileSystem.readAsStringAsync(FILE) : null;
  },
  async write(value) {
    if (FILE.startsWith("file:")) await FileSystem.writeAsStringAsync(FILE, value);
  },
  async remove() {
    if (FILE.startsWith("file:")) await FileSystem.deleteAsync(FILE, { idempotent: true });
  },
};
