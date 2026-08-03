import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import HttpBackend from "i18next-http-backend";
import { getLocales } from "expo-localization";
import { useAuthStore, useLocaleStore } from "@evey/store";
import { OFFLINE_STRINGS } from "./offlineStrings";

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "de", label: "Deutsch" },
  { code: "es", label: "Espa\u00f1ol" },
  { code: "fr", label: "Fran\u00e7ais" },
  { code: "hi", label: "\u0939\u093f\u0928\u094d\u0926\u0940" },
  { code: "zh", label: "\u4e2d\u6587" },
] as const;

function isSupported(code?: string): boolean {
  if (!code) return false;
  return supportedLanguages.some((l) => l.code === code);
}

function detectLanguage(): string {
  const stored = useLocaleStore.getState().locale;
  if (stored) {
    const lang = stored.split("-")[0];
    if (isSupported(lang)) return lang;
  }
  const deviceLang = getLocales()[0]?.languageCode ?? "en";
  if (isSupported(deviceLang)) return deviceLang;
  return "en";
}

export async function initI18n(serverUrl: string) {
  if (i18n.isInitialized) return i18n;

  // Seed the bundled pre-connect strings ONLY when the fetch actually failed:
  // the translations live on the server, so a sleeping server used to leave the
  // very first screen in English whatever the viewer's language (device,
  // 2026-08-03). Seeding unconditionally is worse than the disease - it marks
  // the namespace loaded and the real bundle never lands.
  i18n.on("failedLoading", (lng: string) => seedOfflineStrings(lng));

  await i18n
    .use(HttpBackend)
    .use(initReactI18next)
    .init({
      backend: {
        loadPath: `${serverUrl}/api/v1/i18n/{{lng}}`,
      },
      lng: detectLanguage(),
      fallbackLng: "en",
      ns: ["translation"],
      defaultNS: "translation",
      keySeparator: false,
      interpolation: { escapeValue: false },
    });

  return i18n;
}

// Apply the server's default language on unauthenticated sessions so the
// login/profile-select screens render in the admin-picked language. Mirrors
// mobile's src/i18n/index.ts: once a profile is authenticated, the JWT locale
// claim and any explicit settings choice take over via the existing listeners.
// Deliberately uses a raw fetch (not eveyFetch): this probes a CANDIDATE
// server URL that may differ from the currently configured @evey/api
// baseURL (e.g. while validating a new server in server-connect.tsx), which
// eveyFetch's single module-level baseURL cannot express.
export async function applyServerLanguage(serverUrl: string): Promise<void> {
  if (useAuthStore.getState().accessToken) return;
  try {
    const res = await fetch(`${serverUrl}/api/v1/setup/status`);
    if (!res.ok) return;
    const info = (await res.json()) as { serverLanguage?: string };
    const lang = info.serverLanguage?.split("-")[0];
    if (!isSupported(lang)) return;
    const current = useLocaleStore.getState().locale;
    if (current === lang) return;
    useLocaleStore.getState().setLocale(lang!);
    await i18n.changeLanguage(lang!);
  } catch {
    // setup/status is best-effort; offline boot just keeps the device default.
  }
}

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
