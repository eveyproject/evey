import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, onFocus, onBlur, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, onFocus, onBlur, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { overlay: "rgba(0,0,0,.5)", text: "#fff", accent: "#f00", success: "#0f0" },
  }),
}));

import { SyncplaySessionBadge } from "./SyncplaySessionBadge";

describe("SyncplaySessionBadge", () => {
  it("invites to join a room when not connected", () => {
    const onPress = vi.fn();
    const { getByLabelText } = render(<SyncplaySessionBadge connected={false} onPress={onPress} />);
    fireEvent.click(getByLabelText("Join a room"));
    expect(onPress).toHaveBeenCalled();
  });

  it("shows the connected state and still routes onPress (leave)", () => {
    const onPress = vi.fn();
    const { getByText, getByLabelText } = render(<SyncplaySessionBadge connected onPress={onPress} />);
    expect(getByText("Connected")).toBeTruthy();
    fireEvent.click(getByLabelText("You're synced with the host."));
    expect(onPress).toHaveBeenCalled();
  });

  it("reports focus changes so the player can guard the D-pad select double-fire", () => {
    const onFocusChange = vi.fn();
    const { getByLabelText } = render(
      <SyncplaySessionBadge connected={false} onPress={vi.fn()} onFocusChange={onFocusChange} />,
    );
    fireEvent.focus(getByLabelText("Join a room"));
    expect(onFocusChange).toHaveBeenCalledWith(true);
    fireEvent.blur(getByLabelText("Join a room"));
    expect(onFocusChange).toHaveBeenCalledWith(false);
  });
});
