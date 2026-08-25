import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { lazyZustandStorage, registerPersistedStore } from "./storage";

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
    {
      name: "evey-locale",
      version: 1,
      // Pre-`source` blobs (version 0): back then the stored locale ALWAYS
      // won detection, so re-qualify it as an explicit user pick instead of
      // silently demoting it below the device language on first boot after
      // the update.
      migrate: (persisted) => {
        const p = persisted as Partial<LocaleState> | undefined;
        if (p && p.locale && p.source === undefined) {
          return { ...p, source: "user" as LocaleSource };
        }
        return persisted as LocaleState;
      },
      storage: createJSONStorage(() => lazyZustandStorage),
    }
  )
);

// Late-storage rehydration (React Native): see storage.ts.
registerPersistedStore(useLocaleStore);
