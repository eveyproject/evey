import { describe, it, expect, beforeEach } from "vitest";
import { useLocaleStore } from "./locale";

beforeEach(() => {
  localStorage.clear();
  useLocaleStore.setState({ locale: "en-US" });
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
});
