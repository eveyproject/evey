// From "/legacy": the root's file helpers throw at runtime in
// expo-file-system 55 (see apps/mobile downloads.ts).
import * as FileSystem from "expo-file-system/legacy";

// The server's translation catalogue (~240 KB for one language) was fetched
// and parsed before the first screen at every cold start. The last one is
// kept on disk per server and language: the app starts on it and swaps in
// the fresh one when it lands. A missing or unreadable file is a cache miss.
const DIR = ((FileSystem as unknown as { cacheDirectory?: string }).cacheDirectory ?? "") + "evey-i18n/";

function fileFor(serverUrl: string, lng: string): string {
  // The server URL as a file name: letters, digits and dashes only.
  return `${DIR}${serverUrl.replace(/[^a-z0-9]+/gi, "-")}_${lng}.json`;
}

export async function readCatalogue(serverUrl: string, lng: string): Promise<Record<string, string> | null> {
  if (!DIR.startsWith("file:") || !serverUrl) return null;
  try {
    const path = fileFor(serverUrl, lng);
    const info = await FileSystem.getInfoAsync(path);
    if (!info.exists) return null;
    const parsed = JSON.parse(await FileSystem.readAsStringAsync(path)) as unknown;
    return parsed && typeof parsed === "object" ? (parsed as Record<string, string>) : null;
  } catch {
    return null;
  }
}

export async function writeCatalogue(serverUrl: string, lng: string, catalogue: unknown): Promise<void> {
  if (!DIR.startsWith("file:") || !serverUrl || !catalogue) return;
  try {
    await FileSystem.makeDirectoryAsync(DIR, { intermediates: true }).catch(() => {});
    await FileSystem.writeAsStringAsync(fileFor(serverUrl, lng), JSON.stringify(catalogue));
  } catch {
    /* the next start fetches it again */
  }
}
