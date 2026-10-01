import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "lock" }, children) : null),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  ActivityIndicator: () => React.createElement("div", null, "…"),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k, i18n: { language: "en" } }),
}));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", onAccent: "#fff", overlay: "#000", error: "#f33" }, radii: { md: 8, lg: 12 } }),
}));
const pauseExternal = vi.hoisted(() => vi.fn());
vi.mock("@evey/store", () => ({ useMusicPlayerStore: { getState: () => ({ pauseExternal }) } }));
const lock = vi.hoisted(() => ({ state: { closed: false, nextOpen: undefined as string | undefined }, unlock: vi.fn() }));
vi.mock("@evey/client", () => ({
  useProfileLock: () => ({ ...lock.state, unlock: lock.unlock, recheck: vi.fn() }),
  formatOpening: () => "today 18:00",
}));

import { ProfileLockScreen } from "./ProfileLockScreen";

beforeEach(() => {
  lock.state = { closed: false, nextOpen: undefined };
  lock.unlock.mockReset();
  pauseExternal.mockReset();
  router.replace.mockReset();
});

describe("ProfileLockScreen", () => {
  it("shows nothing while the profile is open", () => {
    render(<ProfileLockScreen />);
    expect(screen.queryByTestId("lock")).toBeNull();
  });

  // Closed: music stops, the screen says when it opens, a wrong PIN says so,
  // and the way out to another profile is there.
  it("locks, stops the music, takes a parent PIN and offers another profile", async () => {
    lock.state = { closed: true, nextOpen: "2026-10-05T16:00:00Z" };
    render(<ProfileLockScreen />);
    expect(screen.getByText("This profile is locked right now")).toBeTruthy();
    expect(screen.getByText("It opens again {{when}}.")).toBeTruthy();
    expect(pauseExternal).toHaveBeenCalled();

    lock.unlock.mockResolvedValueOnce("wrong_pin");
    fireEvent.change(screen.getByLabelText("Parent PIN"), { target: { value: "12a34" } });
    await act(async () => {
      fireEvent.click(screen.getByText("Unlock"));
    });
    expect(lock.unlock).toHaveBeenCalledWith("1234");
    expect(screen.getByText("Wrong parent PIN")).toBeTruthy();

    fireEvent.click(screen.getByText("Switch profile"));
    expect(router.replace).toHaveBeenCalledWith("/profile-select");
  });
});
