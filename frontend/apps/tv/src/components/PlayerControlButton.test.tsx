import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, numberOfLines, ...p }: any) =>
    React.createElement("span", { "data-lines": numberOfLines, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: () => React.createElement("span"),
}));

vi.mock("@evey/theme", () => ({ FONT_SIZE: { lg: 20, xs: 12 } }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { accent: "#f00", onAccent: "#fff" } }),
}));

import { PlayerControlButton } from "./PlayerControlButton";

describe("PlayerControlButton", () => {
  it("keeps its value text and caption on one line inside the fixed-size disc", () => {
    render(
      <PlayerControlButton
        text="1.25x"
        caption="10 s"
        onPress={() => {}}
        accessibilityLabel="speed"
        focusable
      />,
    );
    expect(screen.getByText("1.25x").getAttribute("data-lines")).toBe("1");
    expect(screen.getByText("10 s").getAttribute("data-lines")).toBe("1");
  });
});
