import "@testing-library/jest-dom/vitest";
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import enTranslations from "./en-translations.json";

void i18n.use(initReactI18next).init({
  lng: "en",
  resources: { en: { translation: enTranslations } },
  defaultNS: "translation",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
