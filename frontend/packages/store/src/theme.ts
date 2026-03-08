import { create } from "zustand";
import { persist } from "zustand/middleware";
import { type ThemeName } from "@evey/theme";

interface ThemeState {
  theme: ThemeName;
  customVars: Record<string, string> | null;
  setTheme: (name: ThemeName) => void;
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
