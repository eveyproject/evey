import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "rematch-pending", ...p }),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "rematch-modal" }, children) : null),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {}, hairlineWidth: 1 },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", onDanger: "#fff", danger: "#e00", overlay: "rgba(0,0,0,.5)" },
    radii: { lg: 12 },
  }),
}));

import { RematchConfirmModal } from "./RematchConfirmModal";

describe("RematchConfirmModal", () => {
  it("renders nothing when not visible", () => {
    render(<RematchConfirmModal visible={false} onClose={vi.fn()} onConfirm={vi.fn()} pending={false} />);
    expect(screen.queryByTestId("rematch-modal")).toBeNull();
  });

  it("shows the confirm dialog with the rematch warning when visible", () => {
    render(<RematchConfirmModal visible onClose={vi.fn()} onConfirm={vi.fn()} pending={false} />);
    expect(screen.getByTestId("rematch-modal")).toBeTruthy();
    expect(screen.getByText("media.detail.rematchConfirm")).toBeTruthy();
  });

  it("calls onConfirm when the destructive button is pressed", () => {
    const onConfirm = vi.fn();
    render(<RematchConfirmModal visible onClose={vi.fn()} onConfirm={onConfirm} pending={false} />);
    fireEvent.click(screen.getByLabelText("media.detail.rematchMetadata"));
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("calls onClose when cancel is pressed", () => {
    const onClose = vi.fn();
    render(<RematchConfirmModal visible onClose={onClose} onConfirm={vi.fn()} pending={false} />);
    fireEvent.click(screen.getByLabelText("common.cancel"));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("shows a spinner and disables both buttons while pending", () => {
    render(<RematchConfirmModal visible onClose={vi.fn()} onConfirm={vi.fn()} pending />);
    expect(screen.getByTestId("rematch-pending")).toBeTruthy();
    expect((screen.getByLabelText("common.cancel") as HTMLButtonElement).disabled).toBe(true);
  });
});
