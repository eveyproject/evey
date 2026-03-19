import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import HttpBackend from "i18next-http-backend";

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "zh", label: "中文" },
  { code: "hi", label: "हिन्दी" },
  { code: "es", label: "Español" },
  { code: "fr", label: "Français" },
] as const;

const CACHE_KEY = "evey_i18n_";
const CACHE_TTL = 24 * 60 * 60 * 1000;

function getCached(lng: string): Record<string, string> | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY + lng);
    if (!raw) return null;
    const { data, ts } = JSON.parse(raw);
    if (Date.now() - ts > CACHE_TTL) return null;
    return data;
  } catch {
    return null;
  }
}

function setCache(lng: string, data: Record<string, string>) {
  try {
    localStorage.setItem(
      CACHE_KEY + lng,
      JSON.stringify({ data, ts: Date.now() }),
    );
  } catch {
    /* quota exceeded */
  }
}

// Skip if already initialized (e.g., test setup inits with static translations)
if (!i18n.isInitialized) {
void i18n
  .use(HttpBackend)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    backend: {
      loadPath: "/api/v1/i18n/{{lng}}",
      request: async (
        _options: unknown,
        url: string,
        _payload: unknown,
        callback: (err: Error | null, res: { status: number; data: unknown }) => void,
      ) => {
        const lng = url.split("/").pop() || "en";
        const cached = getCached(lng);

        try {
          const res = await fetch(url);
          if (res.ok) {
            const data = await res.json();
            setCache(lng, data);
            callback(null, { status: 200, data });
          } else if (cached) {
            callback(null, { status: 200, data: cached });
          } else {
            callback(new Error(`HTTP ${res.status}`), {
              status: res.status,
              data: {},
            });
          }
        } catch {
          if (cached) {
            callback(null, { status: 200, data: cached });
          } else {
            callback(new Error("Network error"), { status: 500, data: {} });
          }
        }
      },
    },
    ns: ["translation"],
    defaultNS: "translation",
    fallbackLng: "en",
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      caches: ["localStorage"],
    },
  });
}

export default i18n;
