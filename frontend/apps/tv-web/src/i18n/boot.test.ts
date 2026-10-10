import { describe, expect, it, vi } from "vitest";

// A packaged copy (webOS, file://) with no server chosen yet: no catalogue
// can be fetched, and the server screen was in English whatever the set's
// language.
vi.mock("@evey/api", () => ({ getBaseURL: () => "" }));

describe("before any server is known", () => {
  it("speaks the set's language on the server screen", async () => {
    const { default: i18n } = await import("./index");
    await i18n.changeLanguage("fr");
    expect(i18n.t("mobile.serverConnect.hint")).toBe(
      "Evey tourne sur un serveur que quelqu'un a installé pour vous. Entrez son adresse web, celle que vous ouvririez dans un navigateur.",
    );
    expect(i18n.t("common.clientChoice.stay")).toBe("Rester ici");
    await i18n.changeLanguage("de");
    expect(i18n.t("mobile.serverConnect.connect")).not.toBe("Connect");
  });
});
