import { describe, it, expect, beforeEach } from "vitest";
import { useThemeStore } from "./theme";

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
