// Shared implementation lives in @evey/client (packages/client/src/native/i18n.ts).
// What is this app's: its offline floor, the pre-connect strings in every
// language, seeded when the server catalogue fails to load.
import i18n, { configureNativeI18n } from "@evey/client/native/i18n";
import { OFFLINE_STRINGS } from "./offlineStrings";

// Seed the bundled pre-connect strings ONLY when the fetch actually failed:
// seeding unconditionally marks the namespace loaded and the real bundle never
// lands. A sleeping server used to leave the first screen in English whatever
// the viewer's language (device, 2026-08-03).
configureNativeI18n({ onFailedLoading: (lng) => seedOfflineStrings(lng) });

export { supportedLanguages, initI18n, applyServerLanguage, default } from "@evey/client/native/i18n";

// Also callable directly: the entry screen learns the server is unreachable
// from its own ping, BEFORE i18next times out and fires failedLoading, and the
// first screen would otherwise be painted in English in the meantime.
export function seedOfflineStrings(lng?: string): void {
  const target = lng ?? i18n.language ?? "en";
  const base = target.split("-")[0]!;
  const strings = OFFLINE_STRINGS[base] ?? OFFLINE_STRINGS.en;
  if (!strings) return;
  // overwrite: false - whatever the server already sent stays authoritative.
  i18n.addResourceBundle(base, "translation", strings, true, false);
}
