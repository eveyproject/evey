import { describe, it, expect, beforeEach } from "vitest";
import { useDemoStore } from "./demo";
import { useLocaleStore } from "./locale";
import { useThemeStore } from "./theme";

beforeEach(() => {
  useDemoStore.setState({ demoMode: false });
  useLocaleStore.setState({ locale: "en-US", source: null });
  useThemeStore.setState({ theme: "default", pluginId: null, customVars: null });
});

// On the demo one profile is shared by every visitor, so what the profile says
// about locale and theme is not this visitor's choice: the stores ignore it
// and keep the local pick. Off the demo nothing changes.
describe("demo mode and the profile-following stores", () => {
  it("off the demo, the profile locale and theme apply as before", () => {
    useLocaleStore.getState().setLocale("fr", "profile");
    expect(useLocaleStore.getState()).toMatchObject({ locale: "fr", source: "profile" });
    useThemeStore.getState().applyFromPreferences({ theme: "amoled" });
    expect(useThemeStore.getState().theme).toBe("amoled");
  });

  it("on the demo, a profile-sourced locale is ignored but the visitor's own pick still lands", () => {
    useDemoStore.getState().setDemoMode(true);
    useLocaleStore.getState().setLocale("de", "user");
    useLocaleStore.getState().setLocale("fr", "profile");
    expect(useLocaleStore.getState()).toMatchObject({ locale: "de", source: "user" });
  });

  it("on the demo, the shared profile's theme is ignored and setTheme still works", () => {
    useDemoStore.getState().setDemoMode(true);
    useThemeStore.getState().setTheme("light-blue");
    useThemeStore.getState().applyFromPreferences({ theme: "amoled" });
    expect(useThemeStore.getState().theme).toBe("light-blue");
  });
});
