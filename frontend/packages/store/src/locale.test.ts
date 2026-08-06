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

  // Pre-`source` persisted blobs (version 0): the stored locale always won
  // detection back then, so the migration re-qualifies it as a user pick -
  // otherwise the first boot after the update demotes it below the device
  // language.
  it("migrates a version-0 blob to an explicit user pick", async () => {
    localStorage.setItem("evey-locale", JSON.stringify({ state: { locale: "en" }, version: 0 }));
    await useLocaleStore.persist.rehydrate();
    expect(useLocaleStore.getState().locale).toBe("en");
    expect(useLocaleStore.getState().source).toBe("user");
  });

  it("leaves a current-version blob untouched", async () => {
    localStorage.setItem("evey-locale", JSON.stringify({ state: { locale: "de", source: "server" }, version: 1 }));
    await useLocaleStore.persist.rehydrate();
    expect(useLocaleStore.getState().source).toBe("server");
  });
});
