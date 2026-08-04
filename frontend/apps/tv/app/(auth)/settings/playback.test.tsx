import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, waitFor, within } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, disabled, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: disabled ? undefined : onPress, "aria-label": accessibilityLabel, disabled, ...p },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- expo-router mock -------------------------------------------------------
// Needed since this screen now renders ScreenState on isLoading/isError, and
// ScreenState's "Back" button imports router.back from expo-router - the real
// package's build output isn't transformable by this test's JSX pipeline.
vi.mock("expo-router", () => ({ router: { back: vi.fn() } }));

// --- @tanstack/react-query mock --------------------------------------------------
const invalidateQueriesMock = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      surface: "#111",
      surfaceRaised: "#222",
      border: "#333",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/api mock --------------------------------------------------------------
const currentUserMock = vi.fn();
const listProfilesMock = vi.fn();
const updateProfileMock = vi.fn();
const mutateMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => currentUserMock(...args),
  useUserServiceListProfiles: (...args: unknown[]) => listProfilesMock(...args),
  useUserServiceUpdateProfile: (...args: unknown[]) => updateProfileMock(...args),
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));

import PlaybackSettingsScreen, { resolveLangPref } from "./playback";

const DEFAULT_PROFILE = { id: "prof-1", isDefault: true, preferences: {} };

beforeEach(() => {
  vi.clearAllMocks();
  currentUserMock.mockReturnValue({ data: { id: "user-1" } });
  listProfilesMock.mockReturnValue({ data: { profiles: [DEFAULT_PROFILE] } });
  mutateMock.mockResolvedValue(undefined);
  updateProfileMock.mockReturnValue({ mutate: mutateMock, mutateAsync: mutateMock, isPending: false });
});

describe("PlaybackSettingsScreen (TV)", () => {
  it("renders every preference section", () => {
    render(<PlaybackSettingsScreen />);
    expect(screen.getByText("Max Streaming Bitrate")).toBeTruthy();
    expect(screen.getByText("Preferred Video Codec")).toBeTruthy();
    expect(screen.getByText("Preferred Audio Codec")).toBeTruthy();
    expect(screen.getByText("Max Audio Channels")).toBeTruthy();
    expect(screen.getByText("Default Subtitle Language")).toBeTruthy();
    expect(screen.getByText("Default Audio Language")).toBeTruthy();
    expect(screen.getByText("Subtitle Burn-in")).toBeTruthy();
    expect(screen.getByText("Transcoding")).toBeTruthy();
  });

  it("saves the max bitrate preference on press, merging existing preferences", () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [{ ...DEFAULT_PROFILE, preferences: { default_audio_language: "eng" } }] },
    });
    render(<PlaybackSettingsScreen />);

    fireEvent.click(screen.getByLabelText("8 Mbps"));

    expect(mutateMock).toHaveBeenCalledWith({
      id: "prof-1",
      data: { preferences: { default_audio_language: "eng", max_bitrate_kbps: "8000" } },
    });
  });

  it("saves the preferred video codec on press", () => {
    render(<PlaybackSettingsScreen />);
    fireEvent.click(screen.getByLabelText("HEVC (H.265)"));
    expect(mutateMock).toHaveBeenCalledWith({
      id: "prof-1",
      data: { preferences: { preferred_video_codec: "hevc" } },
    });
  });

  it("toggles allow direct play off via the Disabled row", () => {
    render(<PlaybackSettingsScreen />);
    const disabledRows = screen.getAllByLabelText("Disabled");
    fireEvent.click(disabledRows[0]);
    expect(mutateMock).toHaveBeenCalledWith({
      id: "prof-1",
      data: { preferences: { allow_direct_play: "false" } },
    });
  });

  it("does not save when there is no default profile yet", () => {
    listProfilesMock.mockReturnValue({ data: { profiles: [] } });
    render(<PlaybackSettingsScreen />);
    const bitrateButton = screen.getByLabelText("8 Mbps") as HTMLButtonElement;
    expect(bitrateButton.disabled).toBe(true);
  });

  it("saves the audio language under the shared audio_languages key (not the legacy default_audio_language)", () => {
    render(<PlaybackSettingsScreen />);
    // "Default Subtitle Language" renders before "Default Audio Language", so
    // the second "Français" row (index 1) belongs to the audio language section.
    fireEvent.click(screen.getAllByLabelText("Français")[1]);
    expect(mutateMock).toHaveBeenCalledWith({
      id: "prof-1",
      data: { preferences: { audio_languages: "fr" } },
    });
  });

  it("saves the subtitle language under the shared subtitle_languages key", () => {
    render(<PlaybackSettingsScreen />);
    fireEvent.click(screen.getAllByLabelText("Español")[0]);
    expect(mutateMock).toHaveBeenCalledWith({
      id: "prof-1",
      data: { preferences: { subtitle_languages: "es" } },
    });
  });

  // A list-choice setting: covers the "confirm before applying" contract for
  // a text/radio style preference. OptionRow is redeclared on every render of
  // this screen, so its DOM nodes get replaced on state change - queries are
  // re-run fresh after the click instead of reusing a captured handle.
  describe("preferred video codec, on mutation outcome", () => {
    it("shows the new value once the save succeeds", async () => {
      mutateMock.mockResolvedValueOnce(undefined);
      render(<PlaybackSettingsScreen />);
      expect(within(screen.getAllByLabelText("Auto")[0]).queryByText("✓")).not.toBeNull();

      fireEvent.click(screen.getByLabelText("HEVC (H.265)"));

      await waitFor(() =>
        expect(within(screen.getByLabelText("HEVC (H.265)")).queryByText("✓")).not.toBeNull(),
      );
      expect(within(screen.getAllByLabelText("Auto")[0]).queryByText("✓")).toBeNull();
    });

    it("keeps the original value when the save fails", async () => {
      mutateMock.mockRejectedValueOnce(new Error("network down"));
      render(<PlaybackSettingsScreen />);

      fireEvent.click(screen.getByLabelText("HEVC (H.265)"));

      await waitFor(() => expect(mutateMock).toHaveBeenCalled());
      // let the rejected mutateAsync settle before asserting nothing moved
      await new Promise((r) => setTimeout(r, 0));
      expect(within(screen.getAllByLabelText("Auto")[0]).queryByText("✓")).not.toBeNull();
      expect(within(screen.getByLabelText("HEVC (H.265)")).queryByText("✓")).toBeNull();
    });
  });

  // A boolean toggle: covers the same contract for the on/off style rows.
  describe("allow direct play, on mutation outcome", () => {
    it("shows the new value once the save succeeds", async () => {
      mutateMock.mockResolvedValueOnce(undefined);
      render(<PlaybackSettingsScreen />);
      expect(within(screen.getAllByLabelText("Enabled")[0]).queryByText("✓")).not.toBeNull();

      fireEvent.click(screen.getAllByLabelText("Disabled")[0]);

      await waitFor(() =>
        expect(within(screen.getAllByLabelText("Disabled")[0]).queryByText("✓")).not.toBeNull(),
      );
      expect(within(screen.getAllByLabelText("Enabled")[0]).queryByText("✓")).toBeNull();
    });

    it("keeps the original value when the save fails", async () => {
      mutateMock.mockRejectedValueOnce(new Error("network down"));
      render(<PlaybackSettingsScreen />);

      fireEvent.click(screen.getAllByLabelText("Disabled")[0]);

      await waitFor(() => expect(mutateMock).toHaveBeenCalled());
      await new Promise((r) => setTimeout(r, 0));
      expect(within(screen.getAllByLabelText("Enabled")[0]).queryByText("✓")).not.toBeNull();
      expect(within(screen.getAllByLabelText("Disabled")[0]).queryByText("✓")).toBeNull();
    });
  });
});

describe("resolveLangPref", () => {
  it("prefers the shared key over the legacy one when both are present", () => {
    expect(
      resolveLangPref(
        { audio_languages: "de", default_audio_language: "eng" },
        "audio_languages",
        "default_audio_language",
      ),
    ).toBe("de");
  });

  it("takes only the first code of a multi-language CSV value", () => {
    expect(
      resolveLangPref({ audio_languages: "fr,en,es" }, "audio_languages", "default_audio_language"),
    ).toBe("fr");
  });

  it("falls back to the legacy key, converting its ISO 639-2/B code to ISO 639-1", () => {
    expect(
      resolveLangPref({ default_subtitle_language: "ger" }, "subtitle_languages", "default_subtitle_language"),
    ).toBe("de");
  });

  it("returns an empty string when neither key is set", () => {
    expect(resolveLangPref({}, "audio_languages", "default_audio_language")).toBe("");
  });
});
