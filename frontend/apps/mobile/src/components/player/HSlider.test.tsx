import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React from "react";

vi.mock("expo-secure-store", () => ({ getItemAsync: vi.fn(), setItemAsync: vi.fn() }));
vi.mock("react-native", () => ({
  View: ({ children, accessibilityLabel, onAccessibilityAction, onLayout, onResponderGrant, onResponderMove, onResponderTerminationRequest, ref }: any) =>
    React.createElement(
      "div",
      {
        "aria-label": accessibilityLabel,
        ref: (el: any) => {
          if (!el) return;
          // The panel's bar sits 200 window units in from the left.
          el.measureInWindow = (cb: (x: number, y: number, w: number) => void) => cb(200, 0, 100);
          el.__rn = { onAccessibilityAction, onLayout, onResponderGrant, onResponderMove, onResponderTerminationRequest };
          if (typeof ref === "function") ref(el); else if (ref) ref.current = el;
        },
      },
      children,
    ),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children }: any) => React.createElement("button", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Switch: () => null,
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  useWindowDimensions: () => ({ width: 400, height: 800 }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00" } }) }));

import { HSlider } from "./PlayerSettingsPanel";

// Brightness, subtitle size and background opacity: the same two faults as
// the seek and volume bars - locationX is relative to the thumb the finger
// lands on, and the player stage took any drag past 16 dp.
describe("HSlider", () => {
  it("reads the finger from pageX and keeps the gesture", () => {
    const onChange = vi.fn();
    render(<HSlider value={0.8} min={0} max={1} onChange={onChange} accessibilityLabel="brightness" />);
    const bar = screen.getByLabelText("brightness") as HTMLDivElement & { __rn: Record<string, (e: unknown) => unknown> };
    act(() => { bar.__rn.onLayout({ nativeEvent: { layout: { width: 100 } } }); });
    // Grabbing the thumb at 80%: locationX says 3, pageX says 280.
    act(() => { bar.__rn.onResponderGrant({ nativeEvent: { locationX: 3, pageX: 280 } }); });
    expect(onChange).toHaveBeenLastCalledWith(0.8);
    act(() => { bar.__rn.onResponderMove({ nativeEvent: { locationX: 5, pageX: 230 } }); });
    expect(onChange.mock.calls.at(-1)?.[0]).toBeCloseTo(0.3, 6);
    expect(bar.__rn.onResponderTerminationRequest?.({})).toBe(false);
  });

  it("maps onto a range that does not start at zero", () => {
    const onChange = vi.fn();
    render(<HSlider value={100} min={50} max={200} onChange={onChange} accessibilityLabel="size" />);
    const bar = screen.getByLabelText("size") as HTMLDivElement & { __rn: Record<string, (e: unknown) => unknown> };
    act(() => { bar.__rn.onLayout({ nativeEvent: { layout: { width: 100 } } }); });
    act(() => { bar.__rn.onResponderGrant({ nativeEvent: { pageX: 250 } }); });
    expect(onChange).toHaveBeenLastCalledWith(125);
  });
});
