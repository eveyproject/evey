import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(ListEmptyComponent) : null;
    }
    return React.createElement(
      "div",
      null,
      data.map((item: any, index: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, index) : index },
          renderItem({ item, index }),
        ),
      ),
    );
  },
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

// --- expo-image mock ---------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args) },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
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

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

// --- PluginSlot mock -------------------------------------------------------------
vi.mock("../../../src/features/plugins/PluginSlot", () => ({
  PluginSlot: () => null,
}));

// --- ParentalPINModal mock -------------------------------------------------------
// Rendered as a stand-in "Verify" button so tests can simulate a correct PIN
// entry without exercising the real keypad/eveyFetch flow (covered by its
// own component).
vi.mock("../../../src/components/ParentalPINModal", () => ({
  ParentalPINModal: ({ visible, onVerified, onClose }: any) =>
    visible
      ? React.createElement(
          "div",
          { "data-testid": "pin-modal" },
          React.createElement("button", { onClick: onVerified, "aria-label": "verify-pin" }, "verify"),
          React.createElement("button", { onClick: onClose, "aria-label": "cancel-pin" }, "cancel"),
        )
      : null,
}));

// --- useParentalGate mock ---------------------------------------------------------
const needsPinMock = vi.fn(() => false);
vi.mock("../../../src/hooks/useParentalGate", () => ({
  useParentalGate: () => ({ needsPin: needsPinMock, profileId: "prof-1" }),
}));

// --- @evey/api mock --------------------------------------------------------------
const resumeMock = vi.fn();
const nextUpMock = vi.fn();
const listMediaItemsMock = vi.fn();
const unreadCountMock = vi.fn();
vi.mock("@evey/api", () => ({
  usePlaybackServiceGetResumeItems: (...args: unknown[]) => resumeMock(...args),
  usePlaybackServiceGetNextUp: (...args: unknown[]) => nextUpMock(...args),
  useLibraryServiceListMediaItems: (...args: unknown[]) => listMediaItemsMock(...args),
  useNotificationServiceGetUnreadCount: (...args: unknown[]) => unreadCountMock(...args),
}));

import HomeScreen, { selectHomeFocusTarget } from "./index";

const HERO_ITEM = {
  id: "mi-hero-1",
  title: "Restricted Movie",
  year: 2020,
  genres: ["Drama"],
  officialRating: "R",
};

beforeEach(() => {
  vi.clearAllMocks();
  needsPinMock.mockReturnValue(false);
  resumeMock.mockReturnValue({ data: { items: [], progress: [] }, isLoading: false });
  nextUpMock.mockReturnValue({ data: { items: [] }, isLoading: false });
  listMediaItemsMock.mockReturnValue({ data: { items: [HERO_ITEM] }, isLoading: false });
  unreadCountMock.mockReturnValue({ data: { count: 0 } });
});

describe("selectHomeFocusTarget (initial TV remote focus)", () => {
  it("picks the hero Play button whenever the hero is visible, even if rows also have items", () => {
    expect(selectHomeFocusTarget(true, [true, true, true])).toBe("hero");
  });

  it("falls back to the first non-empty row, in on-screen order, when there is no hero", () => {
    expect(selectHomeFocusTarget(false, [false, true, true])).toBe(1);
    expect(selectHomeFocusTarget(false, [true, false, false])).toBe(0);
  });

  it("returns null when nothing is visible so no element claims initial focus", () => {
    expect(selectHomeFocusTarget(false, [false, false, false])).toBeNull();
  });
});

describe("HomeScreen hero play button (TV parental gate)", () => {
  it("plays directly when the gate does not require a PIN", () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    expect(pushMock).toHaveBeenCalledWith("/player/mi-hero-1");
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });

  it("shows the PIN modal instead of navigating when the gate requires a PIN", () => {
    needsPinMock.mockReturnValue(true);
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    expect(pushMock).not.toHaveBeenCalledWith("/player/mi-hero-1");
    expect(screen.getByTestId("pin-modal")).toBeTruthy();
  });

  it("navigates to the player only after the PIN is verified", () => {
    needsPinMock.mockReturnValue(true);
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    expect(pushMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByLabelText("verify-pin"));
    expect(pushMock).toHaveBeenCalledWith("/player/mi-hero-1");
  });

  it("does not navigate if the PIN modal is cancelled", () => {
    needsPinMock.mockReturnValue(true);
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("media.detail.play"));
    fireEvent.click(screen.getByLabelText("cancel-pin"));
    expect(pushMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId("pin-modal")).toBeNull();
  });
});
