import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import enCommon from "./en/common.yaml";
import enAuth from "./en/auth.yaml";
import enPlayer from "./en/player.yaml";
import enMedia from "./en/media.yaml";
import enAdmin from "./en/admin.yaml";

import zhCommon from "./zh/common.yaml";
import zhAuth from "./zh/auth.yaml";
import zhPlayer from "./zh/player.yaml";
import zhMedia from "./zh/media.yaml";
import zhAdmin from "./zh/admin.yaml";

import hiCommon from "./hi/common.yaml";
import hiAuth from "./hi/auth.yaml";
import hiPlayer from "./hi/player.yaml";
import hiMedia from "./hi/media.yaml";
import hiAdmin from "./hi/admin.yaml";

import esCommon from "./es/common.yaml";
import esAuth from "./es/auth.yaml";
import esPlayer from "./es/player.yaml";
import esMedia from "./es/media.yaml";
import esAdmin from "./es/admin.yaml";

import frCommon from "./fr/common.yaml";
import frAuth from "./fr/auth.yaml";
import frPlayer from "./fr/player.yaml";
import frMedia from "./fr/media.yaml";
import frAdmin from "./fr/admin.yaml";

export const supportedLanguages = [
  { code: "en", label: "English" },
  { code: "zh", label: "\u4E2D\u6587" },
  { code: "hi", label: "\u0939\u093F\u0928\u094D\u0926\u0940" },
  { code: "es", label: "Espa\u00F1ol" },
  { code: "fr", label: "Fran\u00E7ais" },
] as const;

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: { common: enCommon, auth: enAuth, player: enPlayer, media: enMedia, admin: enAdmin },
      zh: { common: zhCommon, auth: zhAuth, player: zhPlayer, media: zhMedia, admin: zhAdmin },
      hi: { common: hiCommon, auth: hiAuth, player: hiPlayer, media: hiMedia, admin: hiAdmin },
      es: { common: esCommon, auth: esAuth, player: esPlayer, media: esMedia, admin: esAdmin },
      fr: { common: frCommon, auth: frAuth, player: frPlayer, media: frMedia, admin: frAdmin },
    },
    defaultNS: "common",
    fallbackLng: "en",
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      caches: ["localStorage"],
    },
  });

export default i18n;
