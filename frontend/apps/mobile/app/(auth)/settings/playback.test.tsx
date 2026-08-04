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
  Switch: ({ value, onValueChange, disabled, ...p }: any) =>
    React.createElement("button", {
      type: "button",
      role: "switch",
      "aria-checked": !!value,
      disabled,
      onClick: disabled ? undefined : () => onValueChange?.(!value),
      ...p,
    }),
  StyleSheet: { create: (s: any) => s },
}));

// --- expo-router -------------------------------------------------------------
vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
}));

// --- react-i18next -----------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
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

      fireEvent.click(screen.getByText("HEVC (H.265)").closest("button")!);

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

      fireEvent.click(screen.getByText("HEVC (H.265)").closest("button")!);

      await waitFor(() => expect(mutateAsyncMock).toHaveBeenCalled());
      // let the rejected mutateAsync settle before asserting nothing moved
      await new Promise((r) => setTimeout(r, 0));
      expect(within(screen.getAllByText("Auto")[0].closest("button")!).queryByText("✓")).not.toBeNull();
      expect(within(screen.getByText("HEVC (H.265)").closest("button")!).queryByText("✓")).toBeNull();
    });
  });

  // A boolean toggle: covers the same contract for a Switch-style row.
  describe("allow direct play, on mutation outcome", () => {
    it("shows the new value once the save succeeds", async () => {
      mutateAsyncMock.mockResolvedValueOnce(undefined);
      render(<PlaybackSettingsScreen />);
      const directPlaySwitch = screen.getAllByRole("switch")[0];
      expect(directPlaySwitch.getAttribute("aria-checked")).toBe("true");

      fireEvent.click(directPlaySwitch);

      await waitFor(() =>
        expect(screen.getAllByRole("switch")[0].getAttribute("aria-checked")).toBe("false"),
      );
      expect(mutateAsyncMock).toHaveBeenCalledWith({ allow_direct_play: "false" });
    });

    it("keeps the original value when the save fails", async () => {
      mutateAsyncMock.mockRejectedValueOnce(new Error("network down"));
      render(<PlaybackSettingsScreen />);
      const directPlaySwitch = screen.getAllByRole("switch")[0];

      fireEvent.click(directPlaySwitch);

      await waitFor(() => expect(mutateAsyncMock).toHaveBeenCalled());
      await new Promise((r) => setTimeout(r, 0));
      expect(screen.getAllByRole("switch")[0].getAttribute("aria-checked")).toBe("true");
    });
  });
});
