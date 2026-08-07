import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, onTouchStart, onTouchMove, ...p }: any) =>
    React.createElement("div", { onTouchStart, onTouchMove, ...p }, children),
  LayoutAnimation: { configureNext: vi.fn(), Presets: { easeInEaseOut: {} } },
  Platform: { OS: "android" },
  UIManager: {},
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111" } }),
}));

vi.mock("./NavRail", () => ({
  NavRail: ({ mode }: any) => React.createElement("div", { "data-testid": "nav-rail", "data-mode": mode }),
}));

import { RailFrame } from "./RailFrame";

function touch(y: number) {
  return { touches: [{ pageX: 0, pageY: y }] };
}

beforeEach(() => vi.clearAllMocks());

describe("RailFrame", () => {
  it("renders the rail initially with the requested mode", () => {
    render(<RailFrame mode="admin">content</RailFrame>);
    expect(screen.getByTestId("nav-rail").getAttribute("data-mode")).toBe("admin");
  });

  it("hides the rail when the user dives into content, shows it again on the way up", () => {
    render(<RailFrame mode="main">content</RailFrame>);
    const content = screen.getByText("content");

    fireEvent.touchStart(content, touch(300));
    fireEvent.touchMove(content, touch(260));
    expect(screen.queryByTestId("nav-rail")).toBeNull();

    fireEvent.touchStart(content, touch(100));
    fireEvent.touchMove(content, touch(140));
    expect(screen.getByTestId("nav-rail")).toBeTruthy();
  });
});
