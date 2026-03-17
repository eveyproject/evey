import { create } from "zustand";
import { persist } from "zustand/middleware";

interface ThemeState {
  theme: string;
  customVars: Record<string, string> | null;
  setTheme: (name: string) => void;
  setCustomVars: (vars: Record<string, string>) => void;
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: "default",
      customVars: null,
      setTheme: (theme) => set({ theme, customVars: null }),
      setCustomVars: (customVars) => set({ customVars }),
    }),
    { name: "evey-theme" }
  )
);
