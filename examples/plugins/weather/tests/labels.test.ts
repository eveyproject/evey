// Tests for i18n translate() helper.
import { describe, it, expect } from "vitest";
import { translate, LABELS } from "../src/i18n/labels";

describe("translate", () => {
  it("returns FR translation for partly_cloudy", () => {
    expect(translate("partly_cloudy", "fr")).toBe("Partiellement nuageux");
  });

  it("returns EN translation for partly_cloudy", () => {
    expect(translate("partly_cloudy", "en")).toBe("Partly cloudy");
  });

  it("returns key as fallback for unknown key", () => {
    expect(translate("inconnu", "fr")).toBe("inconnu");
  });

  it("returns EN when locale is undefined", () => {
    expect(translate("partly_cloudy", undefined)).toBe("Partly cloudy");
  });

  it("slices BCP47 tag: fr-FR → fr", () => {
    expect(translate("partly_cloudy", "fr-FR")).toBe("Partiellement nuageux");
  });

  it("falls back to EN for unknown locale (ja)", () => {
    expect(translate("partly_cloudy", "ja")).toBe("Partly cloudy");
  });

  it("loading label", () => {
    expect(translate("loading", "fr")).toBe("Chargement…");
    expect(translate("loading", "en")).toBe("Loading…");
    expect(translate("loading", "es")).toBe("Cargando…");
    expect(translate("loading", "de")).toBe("Lädt…");
  });

  it("unavailable label", () => {
    expect(translate("unavailable", "fr")).toBe("Indisponible");
    expect(translate("unavailable", "en")).toBe("Unavailable");
    expect(translate("unavailable", "es")).toBe("No disponible");
    expect(translate("unavailable", "de")).toBe("Nicht verfügbar");
  });

  it("thunderstorm translations", () => {
    expect(translate("thunderstorm", "fr")).toBe("Orage");
    expect(translate("thunderstorm", "es")).toBe("Tormenta");
    expect(translate("thunderstorm", "de")).toBe("Gewitter");
  });

  it("heavy_rain translations", () => {
    expect(translate("heavy_rain", "fr")).toBe("Pluie forte");
    expect(translate("heavy_rain", "es")).toBe("Lluvia fuerte");
    expect(translate("heavy_rain", "de")).toBe("Starker Regen");
  });

  it("all keys have all 4 locales defined", () => {
    const locales = ["en", "fr", "es", "de"] as const;
    for (const [key, dict] of Object.entries(LABELS)) {
      for (const loc of locales) {
        expect(dict[loc], `key "${key}" missing locale "${loc}"`).toBeTruthy();
      }
    }
  });
});
