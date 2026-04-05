import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import React from "react";
import type { DLNARenderer } from "../hooks/useCast";

vi.mock("react-native", () => ({
  Platform: { OS: "android", select: (obj: Record<string, unknown>) => obj.android },
  Modal: ({ children, visible }: any) =>
    visible ? React.createElement("div", { "data-testid": "modal" }, children) : null,
  View: ({ children, ...props }: any) => React.createElement("div", props, children),
  Text: ({ children, ...props }: any) => React.createElement("span", props, children),
  Pressable: ({ children, onPress, ...props }: any) =>
    React.createElement("button", { onClick: onPress, ...props }, children),
  ActivityIndicator: (props: any) =>
    React.createElement("div", { "data-testid": "loading", role: "progressbar", ...props }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surface: "#1a1a1a", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", surfaceRaised: "#2a2a2a" },
    radii: { lg: 12, md: 8 },
  }),
}));

import { CastPicker } from "./CastPicker";

const renderers: DLNARenderer[] = [
  { id: "r1", name: "Living Room TV", location: "", controlUrl: "" },
  { id: "r2", name: "Bedroom Speaker", location: "", controlUrl: "" },
];

beforeEach(() => { vi.clearAllMocks(); });

describe("CastPicker", () => {
  it("shows loading state while discovering", () => {
    const discover = vi.fn(() => new Promise<DLNARenderer[]>(() => {}));

    render(
      <CastPicker visible={true} onClose={vi.fn()} onSelect={vi.fn()} discover={discover} />,
    );

    expect(screen.getByTestId("loading")).toBeTruthy();
  });

  it("shows no devices message when empty", async () => {
    const discover = vi.fn().mockResolvedValue([]);

    render(
      <CastPicker visible={true} onClose={vi.fn()} onSelect={vi.fn()} discover={discover} />,
    );

    await waitFor(() => {
      expect(screen.getByText("No devices found")).toBeTruthy();
    });
  });

  it("renders device list after discovery", async () => {
    const discover = vi.fn().mockResolvedValue(renderers);

    render(
      <CastPicker visible={true} onClose={vi.fn()} onSelect={vi.fn()} discover={discover} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Living Room TV")).toBeTruthy();
      expect(screen.getByText("Bedroom Speaker")).toBeTruthy();
    });
  });

  it("calls onSelect and onClose when device is pressed", async () => {
    const discover = vi.fn().mockResolvedValue(renderers);
    const onSelect = vi.fn();
    const onClose = vi.fn();

    render(
      <CastPicker visible={true} onClose={onClose} onSelect={onSelect} discover={discover} />,
    );

    await waitFor(() => {
      expect(screen.getByText("Living Room TV")).toBeTruthy();
    });

    fireEvent.click(screen.getByText("Living Room TV"));

    expect(onSelect).toHaveBeenCalledWith(renderers[0]);
    expect(onClose).toHaveBeenCalled();
  });

  it("renders nothing when not visible", () => {
    const discover = vi.fn().mockResolvedValue([]);

    const { container } = render(
      <CastPicker visible={false} onClose={vi.fn()} onSelect={vi.fn()} discover={discover} />,
    );

    expect(container.innerHTML).toBe("");
  });
});
