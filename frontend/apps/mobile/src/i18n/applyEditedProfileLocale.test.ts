import { describe, it, expect, vi, beforeEach } from "vitest";

// Editing the ACTIVE profile's language must retarget i18n in the running
// session - the JWT claim only reapplies it at the next login/profile
// switch, which read as "changing the language is broken" (2026-08-28).

const { changeLanguage, i18nMock, device, authState, localeState } = vi.hoisted(() => {
  const changeLanguage = vi.fn().mockResolvedValue(undefined);
  const i18nMock = {
    isInitialized: true,
    language: "de",
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
  const device: { language: string | undefined } = { language: "en" };
  const authState: { accessToken: string | null; profileId: string | null } = {
    accessToken: "tok",
    profileId: "p1",
  };
  const localeState = {
    locale: "de",
    source: "profile" as string | null,
    setLocale: vi.fn((locale: string, source?: string) => {
      localeState.locale = locale;
      localeState.source = source ?? "user";
    }),
  };
  return { changeLanguage, i18nMock, device, authState, localeState };
});

vi.mock("i18next", () => ({ default: i18nMock }));
vi.mock("react-i18next", () => ({ initReactI18next: {} }));
vi.mock("i18next-http-backend", () => ({ default: {} }));
vi.mock("expo-localization", () => ({
  getLocales: () => [{ languageCode: device.language }],
}));
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => authState },
  useLocaleStore: { getState: () => localeState },
}));

import { applyEditedProfileLocale } from "./index";

beforeEach(() => {
  vi.clearAllMocks();
  device.language = "en";
  authState.profileId = "p1";
  localeState.locale = "de";
  localeState.source = "profile";
});

describe("applyEditedProfileLocale", () => {
  it("applies a supported locale immediately when the edited profile is the active one", async () => {
    await applyEditedProfileLocale({ id: "p1" }, "fr");

    expect(localeState.setLocale).toHaveBeenCalledWith("fr", "profile");
    expect(changeLanguage).toHaveBeenCalledWith("fr");
  });

  it("strips the region from a BCP 47 locale before applying it", async () => {
    await applyEditedProfileLocale({ id: "p1" }, "fr-FR");

    expect(localeState.setLocale).toHaveBeenCalledWith("fr", "profile");
    expect(changeLanguage).toHaveBeenCalledWith("fr");
  });

  it("does nothing when the edited profile is NOT the active one", async () => {
    await applyEditedProfileLocale({ id: "p2" }, "fr");

    expect(localeState.setLocale).not.toHaveBeenCalled();
    expect(changeLanguage).not.toHaveBeenCalled();
  });

  it("treats the default profile as active when no profile is selected yet", async () => {
    authState.profileId = null;
    await applyEditedProfileLocale({ id: "p9", isDefault: true }, "es");

    expect(localeState.setLocale).toHaveBeenCalledWith("es", "profile");
    expect(changeLanguage).toHaveBeenCalledWith("es");
  });

  it("falls back to the device language when the locale is cleared to Automatic", async () => {
    device.language = "de";
    await applyEditedProfileLocale({ id: "p1" }, "");

    expect(localeState.setLocale).toHaveBeenCalledWith("de", "server");
    expect(changeLanguage).toHaveBeenCalledWith("de");
  });

  it("falls back to English on Automatic when the device language is unsupported", async () => {
    device.language = "ko";
    await applyEditedProfileLocale({ id: "p1" }, "");

    expect(localeState.setLocale).toHaveBeenCalledWith("en", "server");
    expect(changeLanguage).toHaveBeenCalledWith("en");
  });
});
