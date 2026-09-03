import "@testing-library/jest-dom/vitest";
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { vi } from "vitest";

// jsdom doesn't provide ResizeObserver
(globalThis as Record<string, unknown>).ResizeObserver = class {
  constructor(_cb: ResizeObserverCallback) {}
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

// Mock react-virtual so virtualized components render all items in tests
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: (opts: { count: number; estimateSize: (i: number) => number; scrollMargin?: number }) => {
    const size = (i: number) => {
      try { return opts.estimateSize(i); } catch { return 50; }
    };
    let offset = 0;
    const items = Array.from({ length: opts.count }, (_, i) => {
      const s = size(i);
      const item = { index: i, key: i, start: offset, size: s };
      offset += s;
      return item;
    });
    return {
      getVirtualItems: () => items,
      getTotalSize: () => offset,
      measurementsCache: items,
      measureElement: () => {},
      options: { scrollMargin: opts.scrollMargin ?? 0 },
    };
  },
}));

// The English catalogue the tests render with. This used to try a live fetch
// of the dev server's catalogue first, "always in sync" - it never succeeded
// (self-signed TLS; `rejectUnauthorized` is not a Node fetch option), so every
// run, local or CI, silently used this file while it drifted to 1166 missing
// keys (2026-09-01). The file is now the one deterministic source and the Go
// test internal/pkg/i18n TestWebFixtureMatchesEnglishYAML keeps it equal to
// i18n/en; regenerate with `python3 scripts/i18n-fixture.py`.
const translations: Record<string, string> = (await import("./en-translations.json")).default;

void i18n.use(initReactI18next).init({
  lng: "en",
  resources: { en: { translation: translations } },
  defaultNS: "translation",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
