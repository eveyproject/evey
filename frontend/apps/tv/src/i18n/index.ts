import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import HttpBackend from "i18next-http-backend";
import { getLocales } from "expo-localization";
import { useLocaleStore } from "@evey/store";

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "zh", label: "\u4e2d\u6587" },
  { code: "hi", label: "\u0939\u093f\u0928\u094d\u0926\u0940" },
  { code: "es", label: "Espa\u00f1ol" },
  { code: "fr", label: "Fran\u00e7ais" },
] as const;

function detectLanguage(): string {
  const stored = useLocaleStore.getState().locale;
  if (stored) {
    const lang = stored.split("-")[0];
    if (supportedLanguages.some((l) => l.code === lang)) return lang;
  }
  const deviceLang = getLocales()[0]?.languageCode ?? "en";
  if (supportedLanguages.some((l) => l.code === deviceLang)) return deviceLang;
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

export default i18n;
