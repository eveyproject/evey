import { describe, it, expect, vi } from "vitest";

// The boot used to pick the first language before SecureStore had handed the
// persisted locale back: i18next initialised on the device language, fetched
// that catalogue, then applyServerLanguage switched to the viewer's own pick
// and fetched a second one - two bundles and a flash of the wrong language on
// every cold start of a profile whose language differs from the set's.

const i18nMock = {
  isInitialized: false,
  language: "en",
  services: { backendConnector: { backend: { options: { loadPath: "" } } } },
  hasResourceBundle: () => true,
  addResourceBundle: vi.fn(),
  reloadResources: vi.fn().mockResolvedValue(undefined),
  on: vi.fn(),
  use() { return this; },
  init: vi.fn().mockResolvedValue(undefined),
  changeLanguage: vi.fn().mockResolvedValue(undefined),
};

let localeState: { locale: string | null; source: string | null } = { locale: null, source: null };
let hydrated = false;
const hydrationListeners: (() => void)[] = [];
const persist = {
  hasHydrated: () => hydrated,
  onFinishHydration: (cb: () => void) => {
    hydrationListeners.push(cb);
    return () => {};
  },
};

vi.mock("i18next", () => ({ default: i18nMock }));
vi.mock("react-i18next", () => ({ initReactI18next: {} }));
vi.mock("i18next-http-backend", () => ({ default: {} }));
vi.mock("expo-localization", () => ({ getLocales: () => [{ languageCode: "en" }] }));
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => ({ accessToken: null }), persist },
  useLocaleStore: { getState: () => ({ ...localeState, setLocale: vi.fn() }), persist },
}));

describe("initI18n on a cold start", () => {
  it("waits for the persisted locale before choosing the first catalogue", async () => {
    const { initI18n } = await import("./index");
    const done = initI18n("http://host:9091");
    // SecureStore answers a moment later with the viewer's own pick.
    await Promise.resolve();
    localeState = { locale: "fr", source: "user" };
    hydrated = true;
    for (const cb of hydrationListeners.splice(0)) cb();
    await done;

    expect(i18nMock.init).toHaveBeenCalledTimes(1);
    expect(i18nMock.init.mock.calls[0]![0]).toMatchObject({ lng: "fr" });
  });
});
