import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender } from "@testing-library/react";
import type { ReactElement } from "react";
import { vi } from "vitest";

/**
 * What a screen test needs: a query client that never retries (a failing
 * query must surface at once, not after four backoffs) and translations that
 * answer with the fallback the screen passed, or the key.
 */
export function renderScreen(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } });
  return { client, ...rtlRender(<QueryClientProvider client={client}>{ui}</QueryClientProvider>) };
}

/** The spatial navigation library is not under test: every box is focusable. */
export function mockFocus() {
  vi.mock("../tv/focus", () => ({
    FocusRoot: ({ children, className }: { children?: unknown; className?: string }) =>
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (<div className={className}>{children as any}</div>),
    useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
    useTvKeys: () => {},
    setFocus: () => {},
  }));
}

/** Translations: the fallback a screen passed, else the key itself. */
export const i18nStub = {
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => {
      if (typeof fallback === "string") return fallback;
      if (fallback && typeof fallback === "object" && "defaultValue" in (fallback as Record<string, unknown>)) {
        return String((fallback as Record<string, unknown>).defaultValue);
      }
      return key;
    },
    i18n: { language: "en", changeLanguage: () => Promise.resolve() },
  }),
};
