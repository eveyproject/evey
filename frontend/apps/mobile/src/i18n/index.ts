import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import HttpBackend from "i18next-http-backend";
import { getLocales } from "expo-localization";
import { useAuthStore, useLocaleStore } from "@evey/store";

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

export async function initI18n(serverUrl: string) {
  if (i18n.isInitialized) return i18n;

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
