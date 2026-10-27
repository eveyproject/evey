import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

function flat(style: unknown): Record<string, unknown> {
  if (Array.isArray(style)) return Object.assign({}, ...style.map(flat));
  return style && typeof style === "object" ? (style as Record<string, unknown>) : {};
}
vi.mock("react-native", () => ({
  Pressable: ({ children, onPress, onFocus, onBlur, disabled, accessibilityLabel, accessibilityState, style }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, onFocus, onBlur, disabled, "aria-label": accessibilityLabel, "data-state": JSON.stringify(accessibilityState ?? null), "data-border": String(flat(style).borderColor), "data-zoom": JSON.stringify(flat(style).transform ?? null), "data-width": String(flat(style).width ?? "") },
      children,
    ),
  // The focus ring: a child drawn over the card, carrying the accent colour.
  View: ({ style }: any) => React.createElement("div", { "data-testid": "ring", "data-ring": String(flat(style).borderColor) }),
  StyleSheet: { create: (s: any) => s, flatten: flat },
}));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#88f" } }) }));
const lite = vi.hoisted(() => ({ on: false }));
vi.mock("../lib/liteMode", () => ({ useLiteMode: () => lite.on }));

import { FocusableCard } from "./FocusableCard";

// The card paints the accent ring only while the remote sits on it and tells
// the caller so; disabled stops the press AND reaches the screen reader,
// merged with whatever selected state the caller passed.
describe("FocusableCard", () => {
  it("rings on focus, reports focus and blur, and fires the press", () => {
    const onPress = vi.fn();
    const onFocus = vi.fn();
    const onBlur = vi.fn();
    render(<FocusableCard onPress={onPress} onFocus={onFocus} onBlur={onBlur} accessibilityLabel="Play">x</FocusableCard>);
    const btn = screen.getByLabelText("Play");
    // The card's own border stays transparent: the ring is an overlay above
    // the children, so a poster as wide as the card cannot cover it.
    expect(btn.getAttribute("data-border")).toBe("transparent");
    expect(screen.queryByTestId("ring")).toBeNull();
    fireEvent.focus(btn);
    expect(btn.getAttribute("data-border")).toBe("transparent");
    expect(screen.getByTestId("ring").getAttribute("data-ring")).toBe("#88f");
    expect(onFocus).toHaveBeenCalled();
    fireEvent.blur(btn);
    expect(screen.queryByTestId("ring")).toBeNull();
    expect(onBlur).toHaveBeenCalled();
    fireEvent.click(btn);
    expect(onPress).toHaveBeenCalled();
  });

  it("zooms the focused card, except in light mode where the ring alone marks it", () => {
    lite.on = false;
    render(<FocusableCard accessibilityLabel="Full">x</FocusableCard>);
    const full = screen.getByLabelText("Full");
    fireEvent.focus(full);
    expect(full.getAttribute("data-zoom")).toBe(JSON.stringify([{ scale: 1.05 }]));
    lite.on = true;
    render(<FocusableCard accessibilityLabel="Lite">x</FocusableCard>);
    const light = screen.getByLabelText("Lite");
    fireEvent.focus(light);
    expect(screen.getAllByTestId("ring").length).toBeGreaterThan(0);
    expect(light.getAttribute("data-zoom")).toBe("null");
    lite.on = false;
  });

  it("disabled blocks the press and is merged into the accessibility state", () => {
    const onPress = vi.fn();
    render(<FocusableCard onPress={onPress} disabled accessibilityState={{ selected: true }} accessibilityLabel="Busy">x</FocusableCard>);
    const btn = screen.getByLabelText("Busy") as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(JSON.parse(btn.getAttribute("data-state")!)).toEqual({ selected: true, disabled: true });
    fireEvent.click(btn);
    expect(onPress).not.toHaveBeenCalled();
    render(<FocusableCard accessibilityLabel="Plain">x</FocusableCard>);
    expect(screen.getByLabelText("Plain").getAttribute("data-state")).toBe("null");
  });

  it("adds the ring around a numeric size, so content of that size sits inside it", () => {
    render(<FocusableCard accessibilityLabel="Sized" style={{ width: 165, height: 250 }}>x</FocusableCard>);
    expect(screen.getByLabelText("Sized").getAttribute("data-width")).toBe("171");
    render(<FocusableCard accessibilityLabel="Free" style={{ padding: 4 }}>x</FocusableCard>);
    expect(screen.getByLabelText("Free").getAttribute("data-width")).toBe("");
  });
});
