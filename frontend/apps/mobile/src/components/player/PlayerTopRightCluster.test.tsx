import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name, color }: any) => React.createElement("i", { "data-icon": name, "data-color": color }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: string) => f ?? k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00" } }) }));
vi.mock("../../hooks/useChromecast", () => ({ CastButtonNative: () => React.createElement("div", { "data-testid": "native-cast" }) }));
vi.mock("@evey/theme", () => ({ ICON_SIZE: { lg: 24 } }));

import { PlayerTopRightCluster } from "./PlayerTopRightCluster";

// Watch-together reads as "leave" and lights up while connected; the cast
// button (with the invisible native discovery button beside it) exists only
// when a receiver can be found and reads "stop" while casting; "play on"
// appears only when the caller wires it.
describe("PlayerTopRightCluster", () => {
  it("reflects the sync and cast states in labels and colours", () => {
    const h = { onCast: vi.fn(), onSyncPress: vi.fn(), onPlayOnPress: vi.fn() };
    const { rerender } = render(<PlayerTopRightCluster syncConnected={false} castConnected={false} castAvailable {...h} />);
    fireEvent.click(screen.getByLabelText("Watch Together"));
    expect(h.onSyncPress).toHaveBeenCalled();
    fireEvent.click(screen.getByLabelText("player.controls.cast"));
    expect(h.onCast).toHaveBeenCalled();
    fireEvent.click(screen.getByLabelText("player.playOn.button"));
    expect(h.onPlayOnPress).toHaveBeenCalled();
    expect(screen.getByTestId("native-cast")).toBeTruthy();
    expect(document.querySelector("[data-icon='people']")?.getAttribute("data-color")).toBe("#fff");

    rerender(<PlayerTopRightCluster syncConnected castConnected castAvailable {...h} />);
    expect(screen.getByLabelText("Leave")).toBeTruthy();
    expect(screen.getByLabelText("player.controls.stopCast")).toBeTruthy();
    expect(document.querySelector("[data-icon='people']")?.getAttribute("data-color")).toBe("#f00");
    expect(document.querySelector("[data-icon='tv-outline']")?.getAttribute("data-color")).toBe("#f00");
  });

  it("hides cast without a receiver and play-on without a handler", () => {
    render(<PlayerTopRightCluster syncConnected={false} castConnected={false} castAvailable={false} onCast={vi.fn()} onSyncPress={vi.fn()} />);
    expect(screen.queryByLabelText("player.controls.cast")).toBeNull();
    expect(screen.queryByTestId("native-cast")).toBeNull();
    expect(screen.queryByLabelText("player.playOn.button")).toBeNull();
    expect(screen.getByLabelText("Watch Together")).toBeTruthy();
  });
});
