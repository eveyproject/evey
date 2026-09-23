import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import HttpBackend from "i18next-http-backend";
import { getBaseURL } from "@evey/api";
import fallbackEn from "./fallback.en.json";

// The catalogue comes from the server (the same /api/v1/i18n the web app
// reads), but this client boots before it knows a server, and a packaged
// copy runs from the TV's storage: the English floor is embedded (generated
// by scripts/i18n-fixture.py, never edited by hand) so no screen ever paints
// raw keys. Once a server is known, its catalogue takes over.
export const supportedLanguages = ["en", "de", "es", "fr", "hi", "zh"] as const;

const LOCALE_KEY = "evey-tv-locale";

function savedLocale(): string | null {
  try {
    return localStorage.getItem(LOCALE_KEY);
  } catch {
    return null;
  }
}

export function saveLocale(lng: string): void {
  try {
    localStorage.setItem(LOCALE_KEY, lng);
  } catch {
    /* storage refused: the choice lasts for the session */
  }
}

function initialLocale(): string {
  const saved = savedLocale();
  if (saved) return saved;
  const nav = (typeof navigator !== "undefined" ? navigator.language : "en").slice(0, 2).toLowerCase();
  return (supportedLanguages as readonly string[]).includes(nav) ? nav : "en";
}

if (!i18n.isInitialized) {
  void i18n
    .use(HttpBackend)
    .use(initReactI18next)
    .init({
      lng: initialLocale(),
      partialBundledLanguages: true,
      resources: { en: { translation: fallbackEn } },
      backend: {
        // Resolved at request time: the server is chosen after boot.
        loadPath: () => `${getBaseURL()}/api/v1/i18n/{{lng}}`,
        request: async (
          _options: unknown,
          url: string,
          _payload: unknown,
          callback: (err: Error | null, res: { status: number; data: unknown }) => void,
        ) => {
          if (!getBaseURL()) {
            callback(null, { status: 200, data: {} });
            return;
          }
          try {
            const res = await fetch(url);
            if (res.ok) callback(null, { status: 200, data: await res.json() });
            else callback(new Error(`HTTP ${res.status}`), { status: res.status, data: {} });
          } catch {
            callback(new Error("Network error"), { status: 500, data: {} });
          }
        },
      },
      ns: ["translation"],
      defaultNS: "translation",
      fallbackLng: "en",
      keySeparator: false,
      interpolation: { escapeValue: false },
    });
}

/**
 * Once the server is known, pull its catalogue for the active language. A
 * reload emits no languageChanged, the only event the screens redraw on, so
 * the screen on show (sign-in, the client question) kept the English floor
 * until the next navigation: the language is re-applied once it has arrived.
 */
export function reloadCatalogue(): void {
  if (!getBaseURL()) return;
  void i18n.reloadResources().then(() => i18n.changeLanguage(i18n.language));
}

export default i18n;
