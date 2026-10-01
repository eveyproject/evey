import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  ActivityIndicator: () => React.createElement("div", null, "loading"),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
  AccessibilityInfo: { announceForAccessibility: vi.fn() },
}));
vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
}));
const router = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, f?: unknown) => (typeof f === "string" ? f : _k), i18n: { language: "en" } }),
}));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", overlay: "#000", border: "#333", error: "#f33", onAccent: "#fff" }, radii: { md: 8, lg: 12 } }),
}));
vi.mock("../hooks/useParentalGate", () => ({ markUnlocked: vi.fn() }));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: "kid" }) }));
const queue = vi.hoisted(() => ({ setState: vi.fn() }));
vi.mock("../stores/musicQueue", () => ({ useMusicQueueStore: queue }));
const lock = vi.hoisted(() => ({ state: { closed: false, nextOpen: undefined as string | undefined }, unlock: vi.fn() }));
vi.mock("@evey/client", () => ({
  useProfileLock: () => ({ ...lock.state, unlock: lock.unlock, recheck: vi.fn() }),
  formatOpening: () => "today 18:00",
  verifyParentalPin: vi.fn(),
}));

import { ProfileLockScreen } from "./ProfileLockScreen";

beforeEach(() => {
  lock.state = { closed: false, nextOpen: undefined };
  lock.unlock.mockReset();
  queue.setState.mockReset();
  router.replace.mockReset();
});

// On the TV the lock is driven by the remote: the parent PIN goes through the
// keypad to the opening-hours unlock (not the profile-switch PIN check).
describe("TV ProfileLockScreen", () => {
  it("locks, stops the music, and unlocks through the keypad", async () => {
    lock.state = { closed: true, nextOpen: "2026-10-05T16:00:00Z" };
    render(<ProfileLockScreen />);
    expect(screen.getAllByText("This profile is locked right now").length).toBeGreaterThan(0);
    expect(queue.setState).toHaveBeenCalledWith({ isPlaying: false });

    lock.unlock.mockResolvedValueOnce("wrong_pin");
    fireEvent.click(screen.getAllByLabelText("Parent PIN")[0]);
    for (const d of "1234") fireEvent.click(screen.getByLabelText(d));
    await waitFor(() => expect(lock.unlock).toHaveBeenCalledWith("1234"));
    await waitFor(() => expect(screen.getByText("Wrong parent PIN")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Switch profile"));
    expect(router.replace).toHaveBeenCalledWith("/profile-select");
  });

  it("is absent while the profile is open", () => {
    render(<ProfileLockScreen />);
    expect(screen.queryByText("This profile is locked right now")).toBeNull();
  });
});
