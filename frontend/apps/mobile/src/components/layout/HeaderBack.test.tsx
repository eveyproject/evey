import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
}));
const back = vi.hoisted(() => vi.fn());
vi.mock("expo-router", () => ({ router: { back } }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { text: "#fff" } }) }));

import { HeaderBack } from "./HeaderBack";

// Admin detail screens sit under a navigator whose default header button is
// the drawer burger; this chevron returns to the caller instead and is
// labelled for a screen reader.
describe("HeaderBack (mobile)", () => {
  it("is a labelled back button that returns to the caller", () => {
    render(<HeaderBack />);
    const btn = screen.getByRole("button", { name: "Back" });
    expect(btn.querySelector("[data-icon]")?.getAttribute("data-icon")).toBe("chevron-back");
    fireEvent.click(btn);
    expect(back).toHaveBeenCalledTimes(1);
  });
});
