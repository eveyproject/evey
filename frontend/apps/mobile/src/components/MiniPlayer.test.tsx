import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("expo-image", () => ({ Image: ({ source }: any) => React.createElement("img", { alt: "", src: source?.uri }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k }) }));
const store = vi.hoisted(() => ({ state: { mediaId: "m1", mediaTitle: "Film", isActive: true, mode: "mini", stop: vi.fn() } as Record<string, unknown> }));
vi.mock("@evey/store", () => ({ usePlayerStore: (sel: (s: Record<string, unknown>) => unknown) => sel(store.state) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { surfaceRaised: "#222", border: "#333", surface: "#111", text: "#fff", textMuted: "#888" }, radii: { sm: 4 } }) }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({ Authorization: "Bearer tok" }) }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://evey.local" }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12 }, ICON_SIZE: { lg: 24 } }));

import { MiniPlayer } from "./MiniPlayer";

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  store.state = { mediaId: "m1", mediaTitle: "Film", isActive: true, mode: "mini", stop: vi.fn() };
});
afterEach(() => vi.useRealTimers());

// The strip exists only while a session is active in mini mode; a tap opens
// the player once (a double tap within 800 ms is swallowed - it sits outside
// the navigation stack and cannot rely on focus to reset); the cross stops.
describe("MiniPlayer", () => {
  it("opens the player once per tap window and stops on the cross", () => {
    render(<MiniPlayer />);
    expect(document.querySelector("img")?.getAttribute("src")).toBe("http://evey.local/api/v1/media/m1/artwork/poster?w=120");
    fireEvent.click(screen.getByLabelText("Film"));
    fireEvent.click(screen.getByLabelText("Film"));
    expect(router.push).toHaveBeenCalledTimes(1);
    expect(router.push).toHaveBeenCalledWith("/(auth)/player/m1");
    act(() => vi.advanceTimersByTime(800));
    fireEvent.click(screen.getByLabelText("Film"));
    expect(router.push).toHaveBeenCalledTimes(2);
    fireEvent.click(screen.getByLabelText("Stop playback"));
    expect(store.state.stop).toHaveBeenCalled();
  });

  it("hides when inactive, in full mode, or without a media", () => {
    for (const patch of [{ isActive: false }, { mode: "full" }, { mediaId: undefined }]) {
      store.state = { mediaId: "m1", mediaTitle: "Film", isActive: true, mode: "mini", stop: vi.fn(), ...patch };
      const { container, unmount } = render(<MiniPlayer />);
      expect(container.innerHTML).toBe("");
      unmount();
    }
  });
});
