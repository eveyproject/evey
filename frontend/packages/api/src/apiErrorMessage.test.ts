import { describe, it, expect } from "vitest";
import { apiErrorMessage } from "./apiErrorMessage";

// Stands in for i18next: resolves the two keys it knows, echoes the default
// otherwise - exactly how t(key, {defaultValue}) behaves on a missing key.
const t = (key: string, options?: Record<string, unknown>) =>
  key === "errors.auth.invalid_pin"
    ? "Code PIN incorrect"
    : key === "errors.auth.too_many_attempts"
      ? "Trop de tentatives"
      : key === "errors.general.rate_limited"
        ? `Trop de requetes. Reessaye dans ${String(options?.wait)}.`
        : key === "auth.rateLimit.seconds"
          ? `${String(options?.count)} s`
          : key === "auth.rateLimit.minutes"
            ? `${String(options?.count)} min`
            : String(options?.defaultValue ?? key);

describe("apiErrorMessage", () => {
  it("traduit un message qui EST une cle du catalogue", () => {
    expect(apiErrorMessage({ message: "errors.auth.invalid_pin" }, t, "Echec")).toBe("Code PIN incorrect");
  });

  it("traduit le code machine en priorite sur la prose", () => {
    expect(apiErrorMessage({ message: "invalid pin", errorCode: "errors.auth.invalid_pin" }, t, "Echec"))
      .toBe("Code PIN incorrect");
  });

  it("laisse passer une vraie phrase du serveur telle quelle", () => {
    expect(apiErrorMessage({ message: "disk full" }, t, "Echec")).toBe("disk full");
  });

  it("ne montre JAMAIS une cle brute a l'ecran quand elle est inconnue", () => {
    // La regression beta.38 : "errors.auth.too_many_attempts" affiche tel quel.
    expect(apiErrorMessage({ message: "errors.auth.unknown_key" }, t, "Echec")).toBe("Echec");
  });

  it("retombe sur le fallback du caller quand il n'y a rien", () => {
    expect(apiErrorMessage({}, t, "Echec")).toBe("Echec");
    expect(apiErrorMessage(null, t, "Echec")).toBe("Echec");
    expect(apiErrorMessage(undefined, t, "Echec")).toBe("Echec");
  });

  it("resout la cle que le serveur envoie depuis des mois et que le mobile affichait brute", () => {
    expect(apiErrorMessage({ message: "errors.auth.too_many_attempts" }, t, "Echec")).toBe("Trop de tentatives");
  });

  // Un 429 sans delai est inactionnable (rapport 2026-07-29) : le fetcher
  // attache retryAfterSeconds a chaque erreur, l'affichage doit s'en servir
  // partout, pas seulement sur l'ecran de login mobile.
  it("nomme l'attente d'un 429 quel que soit le message", () => {
    expect(apiErrorMessage({ message: "errors.auth.too_many_attempts", httpStatus: 429, retryAfterSeconds: 42 }, t, "Echec"))
      .toBe("Trop de requetes. Reessaye dans 42 s.");
    expect(apiErrorMessage({ message: "rate limited", httpStatus: 429, retryAfterSeconds: 90 }, t, "Echec"))
      .toBe("Trop de requetes. Reessaye dans 2 min.");
  });

  it("garde le message normal quand le 429 n'annonce aucun delai", () => {
    expect(apiErrorMessage({ message: "errors.auth.too_many_attempts", httpStatus: 429 }, t, "Echec"))
      .toBe("Trop de tentatives");
  });
});
