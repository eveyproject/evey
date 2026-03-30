import { create } from "zustand";
import { persist } from "zustand/middleware";

interface ThemeState {
  theme: string;
  pluginId: string | null;
  customVars: Record<string, string> | null;
  setTheme: (name: string, pluginId?: string) => void;
  setCustomVars: (vars: Record<string, string>) => void;
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set) => ({
      theme: "default",
      pluginId: null,
      customVars: null,
      setTheme: (theme, pluginId) => set({ theme, pluginId: pluginId ?? null, customVars: null }),
      setCustomVars: (customVars) => set({ customVars }),
    }),
    { name: "evey-theme" }
  )
);
