import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

const dims = vi.hoisted(() => ({ width: 800 }));
function flat(style: unknown): Record<string, unknown> {
  if (Array.isArray(style)) return Object.assign({}, ...style.map(flat));
  return style && typeof style === "object" ? (style as Record<string, unknown>) : {};
}
vi.mock("react-native", () => ({
  View: ({ children, style, accessibilityLabel, accessibilityValue, onAccessibilityAction, onLayout, onResponderGrant, onResponderMove }: any) =>
    React.createElement(
      "div",
      {
        "aria-label": accessibilityLabel,
        "data-now": accessibilityValue?.now,
        "data-w": flat(style).width,
        "data-bg": flat(style).backgroundColor,
        "data-left": flat(style).left,
        ref: (el: any) => {
          if (el) el.__rn = { onAccessibilityAction, onLayout, onResponderGrant, onResponderMove };
        },
      },
      children,
    ),
  Text: ({ children, style }: any) => React.createElement("span", { "data-color": flat(style).color }, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
  useWindowDimensions: () => ({ width: dims.width, height: 400 }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00", warning: "#fa0" } }) }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10 }, ICON_SIZE: { lg: 24 } }));

import { PlayerVolumeControl } from "./PlayerVolumeControl";

function mount(volume: number, muted = false) {
  const h = { onToggleMute: vi.fn(), onVolumeChange: vi.fn() };
  render(<PlayerVolumeControl volume={volume} muted={muted} {...h} />);
  const slider = (screen.queryAllByLabelText("player.controls.volume").at(-1) ?? null) as (HTMLDivElement & { __rn: Record<string, (e: unknown) => void> }) | null;
  if (slider) act(() => slider.__rn.onLayout({ nativeEvent: { layout: { width: 80 } } }));
  return { ...h, slider };
}

beforeEach(() => {
  vi.clearAllMocks();
  dims.width = 800;
});

// The bar spans 0-2 with 100% in the middle: past it the fill turns to the
// warning colour and the percentage with it; a drag maps the finger to that
// range against the measured width; a screen-reader step is 10 points; in a
// narrow (portrait) window only the mute icon remains.
describe("PlayerVolumeControl", () => {
  it("shows the level, marks a boost, and maps a drag onto 0-2", () => {
    const { slider, onVolumeChange, onToggleMute } = mount(1.5);
    expect(screen.getByText("150%").getAttribute("data-color")).toBe("#fa0");
    expect(slider!.getAttribute("data-now")).toBe("150");
    expect(document.querySelector("[data-bg='#fa0']")?.getAttribute("data-w")).toBe("25%");
    expect(document.querySelector("[data-left='75%']")).toBeTruthy();
    act(() => slider!.__rn.onResponderGrant({ nativeEvent: { locationX: 20 } }));
    expect(onVolumeChange).toHaveBeenLastCalledWith(0.5);
    act(() => slider!.__rn.onResponderMove({ nativeEvent: { locationX: 200 } }));
    expect(onVolumeChange).toHaveBeenLastCalledWith(2);
    act(() => slider!.__rn.onAccessibilityAction({ nativeEvent: { actionName: "increment" } }));
    expect(onVolumeChange).toHaveBeenLastCalledWith(1.6);
    fireEvent.click(screen.getByLabelText("player.controls.mute"));
    expect(onToggleMute).toHaveBeenCalled();
  });

  it("muted reads as zero with an unmute action, and low levels get the low icon", () => {
    mount(0.8, true);
    expect(screen.getByText("0%")).toBeTruthy();
    expect(screen.getByLabelText("player.controls.unmute")).toBeTruthy();
    expect(document.querySelector("[data-icon='volume-mute']")).toBeTruthy();
    expect(document.querySelector("[data-bg='#fa0']")).toBeNull();
    mount(0.3);
    expect(document.querySelector("[data-icon='volume-low']")).toBeTruthy();
  });

  it("keeps only the mute icon in a narrow window", () => {
    dims.width = 400;
    const { slider } = mount(1);
    expect(slider).toBeNull();
    expect(screen.queryByText("100%")).toBeNull();
    expect(screen.getByLabelText("player.controls.mute")).toBeTruthy();
  });
});
