import { describe, it, expect, beforeEach } from "vitest";
import { useThemeStore } from "@evey/store";

function resetStore() {
  useThemeStore.setState({ theme: "default", customVars: null });
}

describe("useThemeStore", () => {
  beforeEach(resetStore);

  it("defaults to 'default' theme", () => {
    expect(useThemeStore.getState().theme).toBe("default");
    expect(useThemeStore.getState().customVars).toBeNull();
  });

  it("setTheme changes theme and clears customVars", () => {
    useThemeStore.getState().setCustomVars({ "--bg": "#fff" });
    useThemeStore.getState().setTheme("amoled");

    const s = useThemeStore.getState();
    expect(s.theme).toBe("amoled");
    expect(s.customVars).toBeNull();
  });

  it("setTheme accepts all valid theme names", () => {
    for (const name of ["default", "amoled", "light-default", "light-blue"] as const) {
      useThemeStore.getState().setTheme(name);
      expect(useThemeStore.getState().theme).toBe(name);
    }
  });

  it("setCustomVars stores custom variables", () => {
    const vars = { "--accent": "#ff0000", "--bg": "#000" };
    useThemeStore.getState().setCustomVars(vars);
    expect(useThemeStore.getState().customVars).toEqual(vars);
  });
});
