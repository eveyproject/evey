import { describe, it, expect, vi, beforeEach } from "vitest";

// Language precedence decided 2026-08-05: profile > explicit user pick >
// device language > server default. The old applyServerLanguage read
// accessToken BEFORE TV storage rehydration finished, so the server default
// won the boot race against a known profile.

const changeLanguage = vi.fn().mockResolvedValue(undefined);
const i18nMock = {
  isInitialized: true,
  language: "en",
  services: { backendConnector: { backend: { options: { loadPath: "" } } } },
  hasResourceBundle: () => true,
  reloadResources: vi.fn().mockResolvedValue(undefined),
  use() {
    return this;
  },
  init: vi.fn().mockResolvedValue(undefined),
  changeLanguage,
  on: vi.fn(),
};

let deviceLanguage: string | undefined = "en";

const authState: { accessToken: string | null } = { accessToken: null };
const localeState = {
  locale: "en-US",
  source: null as string | null,
  setLocale: vi.fn((locale: string, source?: string) => {
    localeState.locale = locale;
    localeState.source = source ?? "user";
  }),
};

// Simulated zustand persist API: hydration is async on device (TV storage).
let authHydrated = true;
let releaseAuthHydration: (() => void) | null = null;
const authPersist = {
  hasHydrated: () => authHydrated,
  onFinishHydration: (cb: () => void) => {
    releaseAuthHydration = () => {
      authHydrated = true;
      cb();
    };
    return () => {};
  },
};

vi.mock("i18next", () => ({ default: i18nMock }));
vi.mock("react-i18next", () => ({ initReactI18next: {} }));
vi.mock("i18next-http-backend", () => ({ default: {} }));
vi.mock("expo-localization", () => ({
  getLocales: () => [{ languageCode: deviceLanguage }],
}));
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => authState, persist: authPersist },
  useLocaleStore: { getState: () => localeState },
}));

function mockSetupStatus(serverLanguage: string) {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ serverLanguage }),
    })
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  deviceLanguage = "en";
  authState.accessToken = null;
  localeState.locale = "en-US";
  localeState.source = null;
  authHydrated = true;
  releaseAuthHydration = null;
});

describe("applyServerLanguage precedence", () => {
  it("waits for hydration: a profile revealed by late rehydration beats the server default", async () => {
    const { applyServerLanguage } = await import("./index");
    mockSetupStatus("de");
    authHydrated = false;

    const done = applyServerLanguage("http://host:9091");
    // Rehydration lands AFTER the call started: the token and the profile
    // locale become visible only now.
    authState.accessToken = "jwt";
    localeState.locale = "fr-FR";
    localeState.source = "profile";
    expect(releaseAuthHydration).not.toBeNull();
    releaseAuthHydration!();
    await done;

    expect(localeState.setLocale).not.toHaveBeenCalled();
    expect(changeLanguage).toHaveBeenCalledWith("fr");
    expect(changeLanguage).not.toHaveBeenCalledWith("de");
  });

  it("an explicit user pick is never overwritten by the server default", async () => {
    const { applyServerLanguage } = await import("./index");
    mockSetupStatus("de");
    localeState.locale = "fr-FR";
    localeState.source = "user";

    await applyServerLanguage("http://host:9091");

    expect(localeState.setLocale).not.toHaveBeenCalled();
    expect(changeLanguage).toHaveBeenCalledWith("fr");
  });

  it("a supported device language outranks the server default", async () => {
    const { applyServerLanguage } = await import("./index");
    mockSetupStatus("de");
    deviceLanguage = "fr";

    await applyServerLanguage("http://host:9091");

    expect(localeState.setLocale).not.toHaveBeenCalled();
    expect(changeLanguage).toHaveBeenCalledWith("fr");
    expect(changeLanguage).not.toHaveBeenCalledWith("de");
  });

  it("the server default applies as last resort when nothing else is known", async () => {
    const { applyServerLanguage } = await import("./index");
    mockSetupStatus("de");
    deviceLanguage = "xx"; // unsupported
    localeState.locale = "";
    localeState.source = null;

    await applyServerLanguage("http://host:9091");

    expect(localeState.setLocale).toHaveBeenCalledWith("de", "server");
    expect(changeLanguage).toHaveBeenCalledWith("de");
  });

  it("re-checks the stores after the fetch: a login landing mid-flight wins over the server default", async () => {
    const { applyServerLanguage } = await import("./index");
    deviceLanguage = "xx"; // unsupported, so the server-default path is reached
    localeState.locale = "";
    localeState.source = null;
    // The fetch resolves AFTER a profile has logged in and set its locale.
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(() => {
        authState.accessToken = "jwt";
        localeState.locale = "fr-FR";
        localeState.source = "profile";
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ serverLanguage: "de" }) });
      })
    );

    await applyServerLanguage("http://host:9091");

    expect(localeState.setLocale).not.toHaveBeenCalledWith("de", "server");
    expect(changeLanguage).not.toHaveBeenCalledWith("de");
  });

  it("a stored server echo ranks below the device language on re-detection", async () => {
    const { applyServerLanguage } = await import("./index");
    mockSetupStatus("de");
    localeState.locale = "de-DE";
    localeState.source = "server";
    deviceLanguage = "fr";

    await applyServerLanguage("http://host:9091");

    expect(changeLanguage).toHaveBeenCalledWith("fr");
  });
});
