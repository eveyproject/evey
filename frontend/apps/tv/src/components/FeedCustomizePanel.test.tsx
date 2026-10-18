import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Modal: ({ children, visible }: any) => (visible ? children : null),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#aaa", accent: "#88f", border: "#333", onAccent: "#000" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, disabled }: any) =>
    React.createElement(
      "button",
      { onClick: disabled ? undefined : onPress, "aria-label": accessibilityLabel, disabled },
      children,
    ),
}));

vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, base: 14, lg: 18, xl: 22, "2xl": 26 } }));
vi.mock("@evey/store", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const invalidateQueriesMock = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
}));

const updateMutateAsyncMock = vi.fn().mockResolvedValue(undefined);
const resetMutateAsyncMock = vi.fn().mockResolvedValue(undefined);
vi.mock("@evey/api", () => ({
  useFeedServiceUpdateProfileFeedConfig: () => ({ mutateAsync: updateMutateAsyncMock }),
  useFeedServiceResetProfileFeedConfig: () => ({ mutateAsync: resetMutateAsyncMock }),
}));

import { FeedCustomizePanel } from "./FeedCustomizePanel";

const CONFIGS = [
  { feedType: "recommended", enabled: true, displayOrder: 0 },
  { feedType: "most_popular", enabled: true, displayOrder: 1 },
];

beforeEach(() => {
  vi.clearAllMocks();
});

// #14: the web/mobile "Reset to Defaults" action was entirely missing on TV -
// a customization mistake had no way back short of manually re-toggling
// every row.
describe("FeedCustomizePanel reset to defaults", () => {
  it("asks for a second press before resetting anything", () => {
    render(
      <FeedCustomizePanel
        visible
        onClose={() => {}}
        configs={CONFIGS}
        profileId="prof-1"
        libraryNames={{}}
        pluginNames={{}}
      />,
    );
    fireEvent.click(screen.getByLabelText("common.feeds.resetDefaults"));
    expect(resetMutateAsyncMock).not.toHaveBeenCalled();
    expect(screen.getByLabelText("common.feeds.resetConfirm")).toBeTruthy();
  });

  it("resets every row and refreshes the config on the second press", async () => {
    render(
      <FeedCustomizePanel
        visible
        onClose={() => {}}
        configs={CONFIGS}
        profileId="prof-1"
        libraryNames={{}}
        pluginNames={{}}
      />,
    );
    fireEvent.click(screen.getByLabelText("common.feeds.resetDefaults"));
    // The confirm button is locked for ~600ms right after the first press so a
    // second, real press cannot land before the "Confirmer ?" label is read.
    await act(() => new Promise((resolve) => setTimeout(resolve, 650)));
    fireEvent.click(screen.getByLabelText("common.feeds.resetConfirm"));

    await waitFor(() => {
      expect(resetMutateAsyncMock).toHaveBeenCalledWith({ feedType: "recommended" });
      expect(resetMutateAsyncMock).toHaveBeenCalledWith({ feedType: "most_popular" });
      expect(invalidateQueriesMock).toHaveBeenCalledWith({ queryKey: ["/api/v1/feeds/config"] });
    });
  });
});

// Two OK presses fired before the viewer could read "Confirmer ?" used to
// wipe the customization silently - no confirmation was ever shown.
describe("FeedCustomizePanel debounces the reset button", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("locks the button right after the first press, so an immediate second press does nothing", () => {
    render(
      <FeedCustomizePanel
        visible
        onClose={() => {}}
        configs={CONFIGS}
        profileId="prof-1"
        libraryNames={{}}
        pluginNames={{}}
      />,
    );
    fireEvent.click(screen.getByLabelText("common.feeds.resetDefaults"));
    const confirmBtn = screen.getByLabelText("common.feeds.resetConfirm") as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);

    fireEvent.click(confirmBtn);
    expect(resetMutateAsyncMock).not.toHaveBeenCalled();
  });

  it("unlocks the button once the confirmation had time to be read", () => {
    render(
      <FeedCustomizePanel
        visible
        onClose={() => {}}
        configs={CONFIGS}
        profileId="prof-1"
        libraryNames={{}}
        pluginNames={{}}
      />,
    );
    fireEvent.click(screen.getByLabelText("common.feeds.resetDefaults"));
    act(() => {
      vi.advanceTimersByTime(600);
    });
    const confirmBtn = screen.getByLabelText("common.feeds.resetConfirm") as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(false);
  });
});
