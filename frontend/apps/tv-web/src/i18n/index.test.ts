import { describe, expect, it, vi } from "vitest";

vi.mock("@evey/api", () => ({ getBaseURL: () => "https://evey.example.com" }));

describe("the server's catalogue", () => {
  // Measured in prod on 2026-09-23: with a French browser the sign-in screen
  // stayed in English. The catalogue arrived once the server was chosen, but a
  // reload does not emit languageChanged, the only event the screens listen
  // to, so nothing on screen was redrawn until the next navigation.
  it("redraws the screens once it has arrived", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ "auth.login.signIn": "Se connecter" }))));
    const { default: i18n, reloadCatalogue } = await import("./index");
    await i18n.changeLanguage("fr");
    const redraw = vi.fn();
    i18n.on("languageChanged", redraw);

    reloadCatalogue();
    await vi.waitFor(() => expect(redraw).toHaveBeenCalled());
    expect(i18n.t("auth.login.signIn")).toBe("Se connecter");
  });
});
