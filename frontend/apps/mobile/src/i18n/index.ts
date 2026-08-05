import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import HttpBackend from "i18next-http-backend";
import { getLocales } from "expo-localization";
import { useAuthStore, useLocaleStore } from "@evey/store";
// Embedded English floor for the screens a user sees BEFORE any server is
// reachable (server-connect, login, register, forgot-password). Translations
// are normally served by the instance, so an unreachable/unconfigured server
// used to render raw keys like "auth.login.signIn" (a beta tester's first
// impression of the app, 2026-07-29). Regenerate from i18n/en/*.yaml when
// those screens gain keys; the server catalogue always wins once loaded.
import fallbackEn from "./fallback.en.json";

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "de", label: "Deutsch" },
  { code: "es", label: "Español" },
  { code: "fr", label: "Français" },
  { code: "hi", label: "हिन्दी" },
  { code: "zh", label: "中文" },
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

// i18nLoaded reports that translations are actually IN MEMORY - not merely
// that i18next was constructed. A failed HTTP load leaves i18next
// "initialized" with an EMPTY catalogue, and every screen then renders raw
// keys ("auth.login.signIn" on a beta tester's phone, 2026-07-29).
export function i18nLoaded(): boolean {
  return i18n.isInitialized && i18n.hasResourceBundle(i18n.language, "translation");
}

export async function initI18n(serverUrl: string) {
  // Re-entrant on FAILURE: the old `isInitialized` guard made the first
  // attempt final, so a boot with no server (or an offline first launch)
  // condemned the whole session to raw keys. Reloading is cheap and idempotent.
  if (i18n.isInitialized) {
    // Re-point on the PATH, never on i18nLoaded(): the embedded English floor
    // IS a loaded bundle, so that guard answered "loaded" after a first launch
    // with no server and skipped this whole branch. The backend then kept the
    // unusable relative loadPath of that first init, every later fetch failed
    // against it - including the changeLanguage() of applyServerLanguage - and
    // the session stayed on the 74-key floor: English everywhere, and raw key
    // names for anything the floor does not carry. Reproduced on device
    // 2026-08-05 (French only came back after a restart, when the URL was
    // known before the first init).
    const wanted = serverUrl ? `${serverUrl}/api/v1/i18n/{{lng}}` : "";
    const backend = i18n.services.backendConnector.backend;
    if (wanted && backend?.options && backend.options.loadPath !== wanted) {
      backend.options.loadPath = wanted;
      try {
        await i18n.reloadResources();
      } catch {
        /* still unreachable: the embedded fallback keeps the UI readable */
      }
    }
    return i18n;
  }

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
      // partialBundledLanguages + the embedded floor: the HTTP catalogue is
      // still fetched and MERGED OVER these, so a reachable server keeps
      // full, localized text; an unreachable one degrades to English words
      // instead of key names.
      partialBundledLanguages: true,
      resources: { en: { translation: fallbackEn } },
    });

  return i18n;
}

// Apply the server's default language on unauthenticated sessions so the
// login/setup screens render in the admin-picked language. Mirrors the
// web __root.tsx logic: once a user is authenticated, the JWT locale claim
// and any explicit /settings choice take over via the existing listeners.
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

export default i18n;
