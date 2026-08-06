import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { getZustandStorage } from "./storage";

// Who last chose the locale. Precedence (user decision 2026-08-05):
// profile > explicit user pick > device language > server default.
// A "server" echo must never outrank a profile claim or a user pick, so
// consumers need to know where the persisted value came from.
export type LocaleSource = "user" | "profile" | "server";

interface LocaleState {
  locale: string; // BCP 47, e.g. "fr-FR", "en-US"
  source: LocaleSource | null;
  setLocale: (locale: string, source?: LocaleSource) => void;
}

export const useLocaleStore = create<LocaleState>()(
  persist(
    (set) => ({
      locale: typeof navigator !== "undefined" ? (navigator.language || "en-US") : "en-US",
      source: null,
      // Default "user": an unmarked caller is an explicit pick somewhere in
      // the UI, and must not be silently overridden by a server default.
      setLocale: (locale, source) => set({ locale, source: source ?? "user" }),
    }),
    { name: "evey-locale", storage: createJSONStorage(() => getZustandStorage()) }
  )
);
