import { create } from "zustand";
import { persist } from "zustand/middleware";

interface LocaleState {
  locale: string; // BCP 47, e.g. "fr-FR", "en-US"
  setLocale: (locale: string) => void;
}

export const useLocaleStore = create<LocaleState>()(
  persist(
    (set) => ({
      locale: typeof navigator !== "undefined" ? (navigator.language || "en-US") : "en-US",
      setLocale: (locale) => set({ locale }),
    }),
    { name: "evey-locale" }
  )
);
