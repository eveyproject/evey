import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { lazyZustandStorage, registerPersistedStore } from "./storage";

interface ThemeState {
  theme: string;
  pluginId: string | null;
  customVars: Record<string, string> | null;
  setTheme: (name: string, pluginId?: string) => void;
  setCustomVars: (vars: Record<string, string>) => void;
  /** Adopt the theme stored on a profile. No-op when the profile has none. */
  applyFromPreferences: (prefs: Record<string, string> | undefined) => void;
}

// Where the theme lives on the profile, so it follows the viewer from the web
// to the phone to the television instead of being a per-device setting.
export const THEME_PREF_KEY = "theme";
export const THEME_PLUGIN_PREF_KEY = "theme_plugin_id";

/**
 * The preferences map to send when saving the theme.
 *
 * ⚠️ MERGES on purpose. UpdateProfile REPLACES the whole map server-side
 * (adapters/driving/grpcapi/user_handler_profile.go: `p.Preferences =
 * req.Preferences`), so sending just `{theme}` would silently wipe
 * audio_languages, subtitle_languages and every other stored preference.
 */
export function themePreferences(
  current: Record<string, string> | undefined,
  theme: string,
  pluginId?: string | null,
): Record<string, string> {
  const next = { ...current };
  next[THEME_PREF_KEY] = theme;
  if (pluginId) next[THEME_PLUGIN_PREF_KEY] = pluginId;
  else delete next[THEME_PLUGIN_PREF_KEY];
  return next;
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      theme: "default",
      pluginId: null,
      customVars: null,
      setTheme: (theme, pluginId) => set({ theme, pluginId: pluginId ?? null, customVars: null }),
      setCustomVars: (customVars) => set({ customVars }),
      applyFromPreferences: (prefs) => {
        const theme = prefs?.[THEME_PREF_KEY];
        if (!theme) return;
        const pluginId = prefs[THEME_PLUGIN_PREF_KEY] ?? null;
        // Only when it actually differs: writing the same value would still
        // clear customVars and repaint on every profile fetch.
        const state = get();
        if (state.theme === theme && state.pluginId === pluginId) return;
        set({ theme, pluginId, customVars: null });
      },
    }),
    { name: "evey-theme", storage: createJSONStorage(() => lazyZustandStorage) }
  )
);

// Late-storage rehydration (React Native): see storage.ts.
registerPersistedStore(useThemeStore);
