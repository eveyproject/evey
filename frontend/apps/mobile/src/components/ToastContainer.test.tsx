import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

const { announceMock } = vi.hoisted(() => ({ announceMock: vi.fn() }));

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Animated: {
    View: ({ children, ...p }: any) => React.createElement("div", p, children),
    Value: class {
      constructor(_v: number) {}
    },
    timing: () => ({ start: (cb?: () => void) => cb?.() }),
    parallel: () => ({ start: (cb?: () => void) => cb?.() }),
  },
  StyleSheet: { create: (s: any) => s },
  AccessibilityInfo: { announceForAccessibility: announceMock },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => React.createElement("i") }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: unknown) => (typeof f === "string" ? f : k) }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surfaceRaised: "#222", shadow: "#000", text: "#fff", textMuted: "#888", success: "#0f0", error: "#f00", info: "#00f", warning: "#ff0" },
  }),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12 }, ICON_SIZE: { lg: 20 } }));

const { toastsRef } = vi.hoisted(() => ({ toastsRef: { current: [] as unknown[] } }));
vi.mock("@evey/store", () => ({
  useToastStore: (sel: (s: unknown) => unknown) => sel({ toasts: toastsRef.current, removeToast: vi.fn() }),
}));

import { ToastContainer } from "./ToastContainer";

describe("ToastContainer accessibility", () => {
  beforeEach(() => {
    announceMock.mockClear();
    toastsRef.current = [];
  });

  it("announces a toast message on appearance", () => {
    toastsRef.current = [{ id: "1", type: "error", message: "Something went wrong" }];
    render(<ToastContainer />);
    expect(announceMock).toHaveBeenCalledWith("Something went wrong");
  });

  it("renders nothing and announces nothing when there are no toasts", () => {
    toastsRef.current = [];
    render(<ToastContainer />);
    expect(announceMock).not.toHaveBeenCalled();
  });
});
