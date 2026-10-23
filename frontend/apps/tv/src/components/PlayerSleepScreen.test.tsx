import { describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  ActivityIndicator: () => null,
  Pressable: ({ children, onPress, testID, hasTVPreferredFocus, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "data-testid": testID, "data-preferred-focus": String(!!hasTVPreferredFocus), ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f: string) => f }) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: {} }) }));
vi.mock("./FocusableCard", () => ({ FocusableCard: ({ children }: any) => React.createElement("div", null, children) }));

import { PlayerSleepScreen } from "./PlayerStatusScreen";

describe("PlayerSleepScreen", () => {
  it("takes the remote's focus over the whole screen and wakes on OK", () => {
    const onWake = vi.fn();
    render(<PlayerSleepScreen onWake={onWake} />);
    const target = screen.getByTestId("player-sleep-screen");
    expect(target.getAttribute("data-preferred-focus")).toBe("true");
    fireEvent.click(target);
    expect(onWake).toHaveBeenCalledTimes(1);
  });
});
