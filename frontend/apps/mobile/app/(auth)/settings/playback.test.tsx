import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render, fireEvent, screen, waitFor, within } from "@testing-library/react";
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
  Switch: ({ value, onValueChange, disabled, ...p }: any) =>
    React.createElement("button", {
      type: "button",
      role: "switch",
      "aria-checked": !!value,
      disabled,
      onClick: disabled ? undefined : () => onValueChange?.(!value),
      ...p,
    }),
  TextInput: (p: any) => React.createElement("input", p),
  StyleSheet: { create: (s: any) => s },
}));

// --- expo-router -------------------------------------------------------------
vi.mock("../../../src/components/BottomSheet", () => ({ BottomSheet: () => null }));

vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
}));

// --- react-i18next -----------------------------------------------------------
vi.mock("react-i18next", () => ({
  // The second argument is a fallback string or, for an interpolated label,
  // the values object: only a string stands in for the key.
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key), i18n: { language: "en" } }),
}));

// --- @tanstack/react-query ----------------------------------------------------
// The screen calls useMutation directly (not a codegen hook), so mutateAsync
// is stubbed here and every test controls its resolution/rejection.
const invalidateQueriesMock = vi.fn();
const mutateAsyncMock = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
  useMutation: (_opts: any) => ({
    mutate: (patch: unknown) => {
      void mutateAsyncMock(patch);
    },
    mutateAsync: mutateAsyncMock,
    isPending: false,
  }),
}));

// --- ThemeProvider -------------------------------------------------------------
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
    spacing: { xs: 4, sm: 8, md: 16, lg: 24 },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/api -------------------------------------------------------------------
const currentUserMock = vi.fn();
const listProfilesMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => currentUserMock(...args),
  useUserServiceListProfiles: (...args: unknown[]) => listProfilesMock(...args),
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
  eveyFetch: vi.fn(),
}));

import PlaybackSettingsScreen from "./playback";

const DEFAULT_PROFILE = { id: "prof-1", isDefault: true, preferences: {} };

// The save runs after the click and mirrors its outcome into state once
// mutateAsync settles: clicking inside act and flushing the microtask queue
// keeps every state update covered.
async function clickAndSettle(el: Element) {
  await act(async () => {
    fireEvent.click(el);
    await new Promise((r) => setTimeout(r, 0));
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  currentUserMock.mockReturnValue({ data: { id: "user-1" } });
  listProfilesMock.mockReturnValue({ data: { profiles: [DEFAULT_PROFILE] } });
  mutateAsyncMock.mockResolvedValue(undefined);
});

describe("PlaybackSettingsScreen (mobile)", () => {
  // A list-choice setting: covers the "confirm before applying" contract for
  // a text/radio style preference.
  describe("preferred video codec, on mutation outcome", () => {
    it("shows the new value once the save succeeds", async () => {
      mutateAsyncMock.mockResolvedValueOnce(undefined);
      render(<PlaybackSettingsScreen />);
      expect(within(screen.getAllByText("Auto")[0].closest("button")!).queryByText("✓")).not.toBeNull();

      await clickAndSettle(screen.getByText("HEVC (H.265)").closest("button")!);

      await waitFor(() =>
        expect(
          within(screen.getByText("HEVC (H.265)").closest("button")!).queryByText("✓"),
        ).not.toBeNull(),
      );
      expect(within(screen.getAllByText("Auto")[0].closest("button")!).queryByText("✓")).toBeNull();
      expect(mutateAsyncMock).toHaveBeenCalledWith({ preferred_video_codec: "hevc" });
    });

    it("keeps the original value when the save fails", async () => {
      mutateAsyncMock.mockRejectedValueOnce(new Error("network down"));
      render(<PlaybackSettingsScreen />);

      await clickAndSettle(screen.getByText("HEVC (H.265)").closest("button")!);

      await waitFor(() => expect(mutateAsyncMock).toHaveBeenCalled());
      // let the rejected mutateAsync settle before asserting nothing moved
      await new Promise((r) => setTimeout(r, 0));
      expect(within(screen.getAllByText("Auto")[0].closest("button")!).queryByText("✓")).not.toBeNull();
      expect(within(screen.getByText("HEVC (H.265)").closest("button")!).queryByText("✓")).toBeNull();
    });
  });

  // Auto-play next episode: its own section ahead of the transcoding toggles,
  // so its switch is the first one on the screen.
  describe("autoplay next episode, on mutation outcome", () => {
    it("shows the new value once the save succeeds", async () => {
      mutateAsyncMock.mockResolvedValueOnce(undefined);
      render(<PlaybackSettingsScreen />);
      const autoplaySwitch = screen.getAllByRole("switch")[0];
      expect(autoplaySwitch.getAttribute("aria-checked")).toBe("true");

      await clickAndSettle(autoplaySwitch);

      await waitFor(() =>
        expect(screen.getAllByRole("switch")[0].getAttribute("aria-checked")).toBe("false"),
      );
      expect(mutateAsyncMock).toHaveBeenCalledWith({ autoplay_next_episode: "false" });
    });

    it("keeps the original value when the save fails", async () => {
      mutateAsyncMock.mockRejectedValueOnce(new Error("network down"));
      render(<PlaybackSettingsScreen />);
      const autoplaySwitch = screen.getAllByRole("switch")[0];

      await clickAndSettle(autoplaySwitch);

      await waitFor(() => expect(mutateAsyncMock).toHaveBeenCalled());
      await new Promise((r) => setTimeout(r, 0));
      expect(screen.getAllByRole("switch")[0].getAttribute("aria-checked")).toBe("true");
    });
  });

  // The data saver is the device's own setting: it never touches the
  // profile, and the play-method switches it replaced are gone.
  describe("data saver", () => {
    it("stores the level on the device without saving the profile", async () => {
      const { useDataSaverStore } = await import("@evey/store");
      useDataSaverStore.setState({ level: "off" });
      render(<PlaybackSettingsScreen />);
      fireEvent.click(screen.getByText("auth.settings.playback.dataSaverTravel"));
      expect(useDataSaverStore.getState().level).toBe("travel");
      expect(mutateAsyncMock).not.toHaveBeenCalled();
      useDataSaverStore.setState({ level: "off" });
    });

    it("no longer offers the play-method switches", () => {
      render(<PlaybackSettingsScreen />);
      expect(screen.queryByText("Allow direct play")).toBeNull();
      expect(screen.queryByText("Allow transcoding")).toBeNull();
    });
  });
});
