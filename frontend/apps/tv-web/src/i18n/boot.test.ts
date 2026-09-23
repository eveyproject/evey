import { describe, expect, it, vi } from "vitest";

// A packaged copy (webOS, file://) with no server chosen yet: no catalogue
// can be fetched, and the server screen was in English whatever the set's
// language.
vi.mock("@evey/api", () => ({ getBaseURL: () => "" }));

describe("before any server is known", () => {
  it("speaks the set's language on the server screen", async () => {
    const { default: i18n } = await import("./index");
    await i18n.changeLanguage("fr");
    expect(i18n.t("mobile.serverConnect.hint")).toBe("Saisissez l'URL de votre instance Evey auto-hébergée");
    expect(i18n.t("common.clientChoice.stay")).toBe("Rester ici");
    await i18n.changeLanguage("de");
    expect(i18n.t("mobile.serverConnect.connect")).not.toBe("Connect");
  });
});
