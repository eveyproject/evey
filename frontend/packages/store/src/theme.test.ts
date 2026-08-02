import { describe, it, expect, beforeEach } from "vitest";
import { useThemeStore, themePreferences } from "./theme";

beforeEach(() => {
  localStorage.clear();
  useThemeStore.setState({ theme: "default", pluginId: null, customVars: null });
});

describe("useThemeStore", () => {
  it("has default state", () => {
    const state = useThemeStore.getState();
    expect(state.theme).toBe("default");
    expect(state.pluginId).toBeNull();
    expect(state.customVars).toBeNull();
  });

  it("setTheme updates theme and clears customVars", () => {
    useThemeStore.getState().setCustomVars({ "--color-primary": "#ff0000" });
    useThemeStore.getState().setTheme("dark");
    const state = useThemeStore.getState();
    expect(state.theme).toBe("dark");
    expect(state.pluginId).toBeNull();
    expect(state.customVars).toBeNull();
  });

  it("setTheme with pluginId sets pluginId", () => {
    useThemeStore.getState().setTheme("custom", "plugin-abc");
    const state = useThemeStore.getState();
    expect(state.theme).toBe("custom");
    expect(state.pluginId).toBe("plugin-abc");
  });

  it("setTheme without pluginId resets pluginId to null", () => {
    useThemeStore.getState().setTheme("dark", "some-plugin");
    useThemeStore.getState().setTheme("light");
    expect(useThemeStore.getState().pluginId).toBeNull();
  });

  it("setCustomVars updates customVars", () => {
    const vars = { "--color-bg": "#fff", "--color-fg": "#000" };
    useThemeStore.getState().setCustomVars(vars);
    expect(useThemeStore.getState().customVars).toEqual(vars);
  });

  it("setCustomVars does not affect theme or pluginId", () => {
    useThemeStore.getState().setTheme("ocean", "plugin-x");
    useThemeStore.getState().setCustomVars({ "--accent": "blue" });
    const state = useThemeStore.getState();
    expect(state.theme).toBe("ocean");
    expect(state.pluginId).toBe("plugin-x");
  });
});

describe("themePreferences", () => {
  // The trap: UpdateProfile replaces the whole preferences map server-side, so
  // saving the theme with `{theme}` alone wiped audio_languages and friends.
  it("preserve les preferences existantes", () => {
    const next = themePreferences({ audio_languages: "fr,en", subtitle_languages: "fr" }, "midnight");
    expect(next).toEqual({ audio_languages: "fr,en", subtitle_languages: "fr", theme: "midnight" });
  });

  it("part d'une map vide quand le profil n'a rien", () => {
    expect(themePreferences(undefined, "default")).toEqual({ theme: "default" });
  });

  it("enregistre le plugin qui fournit le theme", () => {
    expect(themePreferences({}, "neon", "plg-1")).toEqual({ theme: "neon", theme_plugin_id: "plg-1" });
  });

  // Switching from a plugin theme back to a built-in one must remove the
  // plugin id, or the next read would try to load a theme the plugin no
  // longer provides.
  it("retire le plugin quand on revient a un theme integre", () => {
    expect(themePreferences({ theme: "neon", theme_plugin_id: "plg-1" }, "default")).toEqual({ theme: "default" });
  });
});

describe("applyFromPreferences", () => {
  it("adopte le theme du profil", () => {
    useThemeStore.getState().setTheme("default");
    useThemeStore.getState().applyFromPreferences({ theme: "midnight" });
    expect(useThemeStore.getState().theme).toBe("midnight");
  });

  it("ne touche a rien quand le profil ne porte pas de theme", () => {
    useThemeStore.getState().setTheme("midnight");
    useThemeStore.getState().applyFromPreferences({ audio_languages: "fr" });
    expect(useThemeStore.getState().theme).toBe("midnight");
  });

  it("reprend aussi le plugin fournisseur", () => {
    useThemeStore.getState().applyFromPreferences({ theme: "neon", theme_plugin_id: "plg-1" });
    expect(useThemeStore.getState().pluginId).toBe("plg-1");
  });
});
