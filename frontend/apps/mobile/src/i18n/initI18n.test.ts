import { describe, it, expect, vi, beforeEach } from "vitest";

// The first launch of a fresh install runs initI18n("") - no server is known
// yet - and only later, once the user types a URL, initI18n(url). The catalogue
// must follow that second call. It did not: the guard asked "are translations
// loaded?" and the embedded English floor made the answer yes, so the backend
// kept the unusable relative loadPath of the first init and the whole session
// stayed English with raw key names for anything the floor lacks.
//
// 2026-08-09 (emulator): a SECOND way to the same screen was found - the boot
// resolves the right URL, the single catalogue fetch fails once (or runs
// before the server store hydrates), and NOTHING ever retries: the old
// re-entry guard only reloaded when the loadPath CHANGED. The catalogue state
// now rides i18next's loaded/failedLoading events, and initI18n retries
// whenever the catalogue never actually arrived.

const reloadResources = vi.fn().mockResolvedValue(undefined);
const backendOptions: { loadPath: string } = { loadPath: "" };
const handlers: Record<string, (...a: unknown[]) => void> = {};
const i18nMock = {
  isInitialized: false,
  language: "en",
  services: { backendConnector: { backend: { options: backendOptions } } },
  hasResourceBundle: () => true, // the embedded floor: always "loaded"
  reloadResources,
  on: (evt: string, cb: (...a: unknown[]) => void) => { handlers[evt] = cb; },
  use() { return this; },
  init: vi.fn().mockResolvedValue(undefined),
  changeLanguage: vi.fn().mockResolvedValue(undefined),
};

vi.mock("i18next", () => ({ default: i18nMock }));
vi.mock("react-i18next", () => ({ initReactI18next: {} }));
vi.mock("i18next-http-backend", () => ({ default: {} }));
vi.mock("expo-localization", () => ({ getLocales: () => [{ languageCode: "en" }] }));
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => ({ accessToken: null }) },
  useLocaleStore: { getState: () => ({ locale: null, setLocale: vi.fn() }) },
}));

beforeEach(() => {
  vi.resetModules();
  reloadResources.mockClear();
  i18nMock.init.mockClear();
  backendOptions.loadPath = "";
  i18nMock.isInitialized = false;
  for (const k of Object.keys(handlers)) delete handlers[k];
});

describe("initI18n after a first launch with no server", () => {
  it("re-points the backend at the server and reloads, floor or no floor", async () => {
    const { initI18n } = await import("./index");

    // First launch: no server yet.
    await initI18n("");
    i18nMock.isInitialized = true;
    backendOptions.loadPath = "/api/v1/i18n/{{lng}}"; // what that init leaves behind

    // The user types a server. hasResourceBundle() still answers true.
    await initI18n("http://host:9091");

    expect(backendOptions.loadPath).toBe("http://host:9091/api/v1/i18n/{{lng}}");
    expect(reloadResources).toHaveBeenCalledTimes(1);
  });

  it("does not refetch when the path is right AND the catalogue actually arrived", async () => {
    const { initI18n } = await import("./index");
    await initI18n("http://host:9091"); // registers the event handlers
    i18nMock.isInitialized = true;
    backendOptions.loadPath = "http://host:9091/api/v1/i18n/{{lng}}";
    handlers["loaded"]?.({ en: { translation: true } }); // the HTTP catalogue landed

    await initI18n("http://host:9091");

    expect(reloadResources).not.toHaveBeenCalled();
  });

  it("retries a failed catalogue load even when the path never changed", async () => {
    const { initI18n } = await import("./index");
    await initI18n("http://host:9091");
    i18nMock.isInitialized = true;
    backendOptions.loadPath = "http://host:9091/api/v1/i18n/{{lng}}";
    handlers["failedLoading"]?.("en", "translation", "network down"); // the one fetch of the boot failed

    await initI18n("http://host:9091"); // e.g. the (auth) layout's second chance

    expect(reloadResources).toHaveBeenCalledTimes(1);
  });
});
