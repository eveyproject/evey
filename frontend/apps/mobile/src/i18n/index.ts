// Shared implementation lives in @evey/client (packages/client/src/native/i18n.ts).
// What is this app's: its offline floor.
import { configureNativeI18n } from "@evey/client/native/i18n";
// Embedded English floor for the screens a user sees BEFORE any server is
// reachable (server-connect, login, register, forgot-password): an unreachable
// server used to render raw keys like "auth.login.signIn" (2026-07-29).
// Regenerate from i18n/en/*.yaml when those screens gain keys; the server
// catalogue always wins once loaded.
import fallbackEn from "./fallback.en.json";

configureNativeI18n({ resources: { en: { translation: fallbackEn } } });

export {
  supportedLanguages,
  storeHydrated,
  i18nLoaded,
  initI18n,
  applyServerLanguage,
  applyEditedProfileLocale,
  default,
} from "@evey/client/native/i18n";
