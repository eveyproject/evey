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
      { onClick: onPress, onFocus, onBlur, disabled, "aria-label": accessibilityLabel, "data-state": JSON.stringify(accessibilityState ?? null), "data-border": String(flat(style).borderColor) },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#88f" } }) }));

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
    expect(btn.getAttribute("data-border")).toBe("transparent");
    fireEvent.focus(btn);
    expect(btn.getAttribute("data-border")).toBe("#88f");
    expect(onFocus).toHaveBeenCalled();
    fireEvent.blur(btn);
    expect(btn.getAttribute("data-border")).toBe("transparent");
    expect(onBlur).toHaveBeenCalled();
    fireEvent.click(btn);
    expect(onPress).toHaveBeenCalled();
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
});
