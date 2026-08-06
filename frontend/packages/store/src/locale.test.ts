import { describe, it, expect, beforeEach } from "vitest";
import { useLocaleStore } from "./locale";

beforeEach(() => {
  localStorage.clear();
  useLocaleStore.setState({ locale: "en-US", source: null });
});

describe("useLocaleStore", () => {
  it("has a default locale", () => {
    const { locale } = useLocaleStore.getState();
    expect(typeof locale).toBe("string");
    expect(locale.length).toBeGreaterThan(0);
  });

  it("setLocale updates locale", () => {
    useLocaleStore.getState().setLocale("fr-FR");
    expect(useLocaleStore.getState().locale).toBe("fr-FR");
  });

  it("setLocale can switch back to another locale", () => {
    useLocaleStore.getState().setLocale("de-DE");
    useLocaleStore.getState().setLocale("en-US");
    expect(useLocaleStore.getState().locale).toBe("en-US");
  });

  it("setLocale accepts BCP 47 tags", () => {
    useLocaleStore.getState().setLocale("zh-Hans-CN");
    expect(useLocaleStore.getState().locale).toBe("zh-Hans-CN");
  });

  // Precedence decided 2026-08-05: profile > user pick > device > server
  // default. Consumers rank the stored value by its source, so the store
  // must record who wrote it.
  it("records the source of a locale write", () => {
    useLocaleStore.getState().setLocale("fr-FR", "profile");
    expect(useLocaleStore.getState().source).toBe("profile");
    useLocaleStore.getState().setLocale("de-DE", "server");
    expect(useLocaleStore.getState().source).toBe("server");
  });

  it("an unmarked write counts as an explicit user pick", () => {
    useLocaleStore.getState().setLocale("fr-FR");
    expect(useLocaleStore.getState().source).toBe("user");
  });

  it("starts with no source", () => {
    expect(useLocaleStore.getState().source).toBeNull();
  });
});
