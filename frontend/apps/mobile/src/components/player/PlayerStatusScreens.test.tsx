import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress }: any) => React.createElement("button", { onClick: onPress }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StatusBar: () => null,
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00" } }) }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, base: 14, lg: 18 }, ICON_SIZE: { "3xl": 48 } }));

import { ShedScreen, ErrorScreen, LoadingScreen } from "./PlayerStatusScreens";

// A shed answer says WHY (the server is full, or the file is still being
// prepared) with a retry and a way back; a failed stream offers retry only
// when the caller has something sensible to retry.
describe("PlayerStatusScreens", () => {
  it("shed screens name the reason and offer retry and back", () => {
    const onRetry = vi.fn();
    const onBack = vi.fn();
    const { rerender } = render(<ShedScreen shedReason="too_many_streams" onRetry={onRetry} onBack={onBack} />);
    expect(screen.getByText("player.shed.tooManyStreams.title")).toBeTruthy();
    expect(screen.getByText("player.shed.tooManyStreams.body")).toBeTruthy();
    fireEvent.click(screen.getByText("player.retry"));
    expect(onRetry).toHaveBeenCalled();
    fireEvent.click(screen.getByText("player.goBack"));
    expect(onBack).toHaveBeenCalled();
    rerender(<ShedScreen shedReason="file_not_ready" onRetry={onRetry} onBack={onBack} />);
    expect(screen.getByText("player.shed.fileNotReady.title")).toBeTruthy();
  });

  it("a source too heavy for the server offers the way back, never a retry", () => {
    const onBack = vi.fn();
    render(<ShedScreen shedReason="source_too_heavy" onRetry={vi.fn()} onBack={onBack} />);
    expect(screen.getByText("player.shed.sourceTooHeavy.title")).toBeTruthy();
    expect(screen.queryByText("player.retry")).toBeNull();
    fireEvent.click(screen.getByText("player.goBack"));
    expect(onBack).toHaveBeenCalled();
  });

  it("the error screen offers retry only when given one; loading is a spinner", () => {
    const onBack = vi.fn();
    render(<ErrorScreen onBack={onBack} />);
    expect(screen.getByText("player.failedToLoadStream")).toBeTruthy();
    expect(screen.queryByText("player.retry")).toBeNull();
    fireEvent.click(screen.getByText("player.goBack"));
    expect(onBack).toHaveBeenCalled();
    const onRetry = vi.fn();
    render(<ErrorScreen onBack={vi.fn()} onRetry={onRetry} />);
    fireEvent.click(screen.getByText("player.retry"));
    expect(onRetry).toHaveBeenCalled();
    render(<LoadingScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
  });
});
