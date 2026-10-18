import { describe, it, expect, vi } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: (props: any) => React.createElement("span", { "data-testid": `icon-${props.name}` }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surface: "#111", border: "#333", accent: "#f00" },
  }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { HomeShortcutButton } from "./HomeShortcutButton";

describe("HomeShortcutButton", () => {
  it("calls onPress when activated", () => {
    const onPress = vi.fn();
    render(<HomeShortcutButton onPress={onPress} />);
    fireEvent.click(screen.getByLabelText("common.nav.home"));
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  it("never claims the initial TV focus - the underlying screen owns it", () => {
    render(<HomeShortcutButton onPress={vi.fn()} />);
    const button = screen.getByLabelText("common.nav.home");
    expect(button.getAttribute("data-tv-preferred-focus")).not.toBe("true");
  });
});
