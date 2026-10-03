import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, style }: any) => React.createElement("div", { "data-opacity": Array.isArray(style) ? undefined : style?.opacity, "data-bg": Array.isArray(style) ? style[1]?.backgroundColor : style?.backgroundColor }, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityRole, style }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, role: accessibilityRole, "data-top": style.top, "data-left": style.left }, children),
  StyleSheet: { absoluteFillObject: {} },
}));
vi.mock("expo-image", () => ({ Image: ({ source }: any) => React.createElement("img", { "data-testid": "backdrop", src: source.uri, "data-auth": source.headers?.Authorization }) }));
vi.mock("react-native-svg", () => ({
  default: ({ children }: any) => React.createElement("svg", null, children),
  Defs: ({ children }: any) => React.createElement("defs", null, children),
  LinearGradient: ({ children, x1, y1, x2, y2 }: any) => React.createElement("linearGradient", { "data-v": `${x1},${y1},${x2},${y2}` }, children),
  Rect: ({ fill }: any) => React.createElement("rect", { "data-fill": fill }),
  Stop: ({ stopOpacity, stopColor }: any) => React.createElement("stop", { "data-opacity": stopOpacity, "data-color": stopColor }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { surface: "#111", bg: "#000" } }) }));

import { MediaHeroBackdrop } from "./MediaHeroBackdrop";

// The backdrop is the authenticated artwork when there is one and a plain
// surface otherwise; the darkening is one SVG vertical gradient over the whole hero (24 stops, no
// flat steps) and the back button clears the safe-area insets.
describe("MediaHeroBackdrop", () => {
  it("shows the artwork with the auth header and a full-height gradient", () => {
    render(<MediaHeroBackdrop uri="https://s/backdrop.jpg" headers={{ Authorization: "Bearer x" }} height={300} onBack={() => {}} topInset={0} leftInset={0} />);
    const img = screen.getByTestId("backdrop");
    expect(img.getAttribute("src")).toBe("https://s/backdrop.jpg");
    expect(img.getAttribute("data-auth")).toBe("Bearer x");
    const bands = document.querySelectorAll("stop");
    expect(bands.length).toBe(24);
    expect(document.querySelector("linearGradient")!.getAttribute("data-v")).toBe("0,0,0,1");
    expect(document.querySelector("rect")!.getAttribute("data-fill")).toBe("url(#heroFade)");
    expect(bands[0].getAttribute("data-color")).toBe("#000");
    const first = Number(bands[0].getAttribute("data-opacity"));
    const last = Number(bands[bands.length - 1].getAttribute("data-opacity"));
    expect(first).toBeLessThan(last);
    expect(last).toBeLessThanOrEqual(1);
  });

  it("falls back to a surface without artwork and offsets the back button by the insets", () => {
    const onBack = vi.fn();
    render(<MediaHeroBackdrop height={300} onBack={onBack} topInset={40} leftInset={10} />);
    expect(screen.queryByTestId("backdrop")).toBeNull();
    expect(document.querySelector('[data-bg="#111"]')).toBeTruthy();
    const back = screen.getByRole("button");
    expect(back.getAttribute("aria-label")).toBe("common.back");
    expect(back.getAttribute("data-top")).toBe("48");
    expect(back.getAttribute("data-left")).toBe("18");
    fireEvent.click(back);
    expect(onBack).toHaveBeenCalledTimes(1);
  });
});
