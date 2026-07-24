import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Animated: {
    View: ({ children, style, ...p }: any) => React.createElement("div", { style, ...p }, children),
    Value: class {
      constructor(public _value: number) {}
    },
    timing: () => ({ start: (cb?: () => void) => cb?.() }),
    parallel: (anims: Array<{ start: () => void }>) => ({
      start: (cb?: () => void) => {
        anims.forEach((a) => a.start());
        cb?.();
      },
    }),
  },
  StyleSheet: { create: (s: any) => s },
}));

// --- @expo/vector-icons mock -------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: (props: any) => React.createElement("span", { "data-testid": `icon-${props.name}` }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      text: "#fff",
      textMuted: "#888",
      surfaceRaised: "#222",
      shadow: "#000",
      success: "#0f0",
      error: "#f00",
      warning: "#ff0",
      info: "#00f",
    },
  }),
}));

// --- @evey/store mock ---------------------------------------------------------
const { toastStateMock, removeToastMock } = vi.hoisted(() => ({
  toastStateMock: { toasts: [] as Array<{ id: string; type: string; message: string; duration?: number }> },
  removeToastMock: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  useToastStore: (sel: any) => sel({ toasts: toastStateMock.toasts, removeToast: removeToastMock }),
}));

import { ToastContainer } from "./ToastContainer";

beforeEach(() => {
  vi.clearAllMocks();
  toastStateMock.toasts = [];
});

describe("ToastContainer (TV)", () => {
  it("renders nothing when the toast store is empty", () => {
    const { container } = render(<ToastContainer />);
    expect(container.innerHTML).toBe("");
  });

  it("renders the message of a toast held in the store", () => {
    toastStateMock.toasts = [{ id: "t1", type: "info", message: "Shuffle enabled" }];
    render(<ToastContainer />);
    expect(screen.getByText("Shuffle enabled")).toBeTruthy();
  });
});
