import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

// The follower is what makes a setting saved on one device show on the
// others: it adopts whatever the reloaded profile carries.

const applyFromPreferences = vi.fn();
let demo = false;
vi.mock("@evey/store", () => ({
  isDemoMode: () => demo,
  useThemeStore: (selector: (s: { applyFromPreferences: typeof applyFromPreferences }) => unknown) =>
    selector({ applyFromPreferences }),
}));

type Profile = { id: string; locale?: string; isDefault?: boolean; preferences?: Record<string, string> };
let profile: Profile | undefined;
vi.mock("./useActiveProfile", () => ({
  useActiveProfile: () => ({ profile }),
}));

import { useProfileSettingsFollower } from "./useProfileSettingsFollower";

describe("useProfileSettingsFollower", () => {
  beforeEach(() => {
    applyFromPreferences.mockClear();
    demo = false;
    profile = { id: "p1", locale: "fr", isDefault: true, preferences: { theme: "default" } };
  });

  it("adopts the theme of the profile each time its preferences change", () => {
    const { rerender } = renderHook(() => useProfileSettingsFollower());
    expect(applyFromPreferences).toHaveBeenLastCalledWith({ theme: "default" });

    profile = { ...profile!, preferences: { theme: "amoled" } };
    rerender();
    expect(applyFromPreferences).toHaveBeenLastCalledWith({ theme: "amoled" });
    expect(applyFromPreferences).toHaveBeenCalledTimes(2);
  });

  it("reports a language changed under the session, not the one it started with", () => {
    const onLocale = vi.fn();
    const { rerender } = renderHook(() => useProfileSettingsFollower(onLocale));
    expect(onLocale).not.toHaveBeenCalled();

    profile = { ...profile!, locale: "de" };
    rerender();
    expect(onLocale).toHaveBeenCalledTimes(1);
    expect(onLocale).toHaveBeenCalledWith({ id: "p1", isDefault: true }, "de");

    rerender();
    expect(onLocale).toHaveBeenCalledTimes(1);
  });

  // Switching to another profile is not a language change of the first one:
  // the profile switch has its own path (a new token, a new claim).
  it("stays quiet when the session moves to another profile", () => {
    const onLocale = vi.fn();
    const { rerender } = renderHook(() => useProfileSettingsFollower(onLocale));
    profile = { id: "p2", locale: "en" };
    rerender();
    expect(onLocale).not.toHaveBeenCalled();
  });

  it("leaves the language alone on the demo, where the profile is shared", () => {
    demo = true;
    const onLocale = vi.fn();
    const { rerender } = renderHook(() => useProfileSettingsFollower(onLocale));
    profile = { ...profile!, locale: "de" };
    rerender();
    expect(onLocale).not.toHaveBeenCalled();
  });
});
