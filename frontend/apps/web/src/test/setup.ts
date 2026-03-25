import "@testing-library/jest-dom/vitest";
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { vi } from "vitest";

// jsdom doesn't provide ResizeObserver
(globalThis as Record<string, unknown>).ResizeObserver = class {
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
      measureElement: () => {},
      options: { scrollMargin: opts.scrollMargin ?? 0 },
    };
  },
}));

// Fetch translations from backend (always in sync, no stale JSON file)
const res = await fetch("https://evey.test/api/v1/i18n/en", {
  // Ignore self-signed cert in dev
  // @ts-expect-error -- node fetch option
  rejectUnauthorized: false,
}).catch(() => null);

let translations: Record<string, string> = {};
if (res?.ok) {
  translations = await res.json();
} else {
  // Fallback: load from static file if backend is down
  translations = (await import("./en-translations.json")).default;
}

void i18n.use(initReactI18next).init({
  lng: "en",
  resources: { en: { translation: translations } },
  defaultNS: "translation",
  fallbackLng: "en",
  interpolation: { escapeValue: false },
});
