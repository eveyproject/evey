import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";
import HttpBackend from "i18next-http-backend";

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "de", label: "Deutsch" },
  { code: "es", label: "Español" },
  { code: "fr", label: "Français" },
  { code: "hi", label: "हिन्दी" },
  { code: "zh", label: "中文" },
] as const;

const CACHE_KEY = "evey_i18n_";
const CACHE_TTL = 24 * 60 * 60 * 1000;
const IS_DEV = import.meta.env.DEV;

// A lazily loaded route chunk asking for a key the in-memory catalogue lacks
// means the catalogue predates the bundle (tab left open across a deploy):
// refetch it once instead of showing raw keys until the next reload.
let staleRefetchAt = 0;
const STALE_REFETCH_MIN_GAP = 60 * 1000;
export function refetchOnMissingKey(now = Date.now()): void {
  if (IS_DEV || now - staleRefetchAt < STALE_REFETCH_MIN_GAP) return;
  staleRefetchAt = now;
  void i18n.reloadResources();
}

function getCached(lng: string): Record<string, string> | null {
  if (IS_DEV) return null;
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
  if (IS_DEV) return;
  try {
    localStorage.setItem(
      CACHE_KEY + lng,
      JSON.stringify({ data, ts: Date.now() }),
    );
  } catch {
    /* quota exceeded */
  }
}

// Keep <html lang> in sync with the active language so screen readers and
// browser tooling pick up the right locale. Runs in both fresh-init and
// already-initialized cases (test setup also benefits).
if (typeof document !== "undefined") {
  i18n.on("languageChanged", (lng) => {
    document.documentElement.lang = lng;
  });
}

// Skip if already initialized (e.g., test setup inits with static translations)
if (!i18n.isInitialized) {
void i18n
  .use(HttpBackend)
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    saveMissing: true,
    missingKeyHandler: () => refetchOnMissingKey(),
    backend: {
      loadPath: "/api/v1/i18n/{{lng}}",
      // saveMissing would otherwise POST every missing key to the backend's
      // default /locales/add path; the handler above only refetches.
      addPath: "",
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
    keySeparator: false,
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      caches: ["localStorage"],
    },
  });
}

export default i18n;
