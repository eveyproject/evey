import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Pressable: ({ children, onPress, testID }: any) => React.createElement("button", { onClick: onPress, "data-testid": testID }, children),
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", overlay: "rgba(0,0,0,.5)", border: "#333" } }),
}));
const route = vi.hoisted(() => ({ pathname: "/" }));
vi.mock("expo-router", () => ({ usePathname: () => route.pathname }));
const updates = vi.hoisted(() => ({ pending: false, version: "9.9.9", reloadAsync: vi.fn(async () => {}) }));
vi.mock("expo-updates", () => ({
  useUpdates: () => ({ isUpdatePending: updates.pending, downloadedUpdate: { manifest: { extra: { expoClient: { version: updates.version } } } } }),
  reloadAsync: () => updates.reloadAsync(),
}));
const hook = vi.hoisted(() => ({ dismiss: vi.fn() }));
vi.mock("@evey/client/native/useClientUpdates", () => ({
  useClientUpdates: (_binding: unknown, pending: boolean) => ({ offerRestart: pending, dismiss: hook.dismiss }),
}));
vi.mock("../lib/clientUpdates", () => ({ clientUpdateBinding: {} }));
vi.mock("../../app.json", () => ({ default: { expo: { version: "1.0.0" } } }));

import { ClientUpdatePrompt } from "./ClientUpdatePrompt";

beforeEach(() => {
  vi.clearAllMocks();
  route.pathname = "/";
  updates.pending = false;
  updates.version = "9.9.9";
});

describe("ClientUpdatePrompt", () => {
  it("renders nothing while no bundle is waiting", () => {
    const { container } = render(<ClientUpdatePrompt />);
    expect(container.innerHTML).toBe("");
  });

  it("offers the restart, and restarts on press", () => {
    updates.pending = true;
    const { getByTestId } = render(<ClientUpdatePrompt />);

    fireEvent.click(getByTestId("client-update-restart"));
    expect(updates.reloadAsync).toHaveBeenCalledTimes(1);
  });

  it("lets the viewer put it off", () => {
    updates.pending = true;
    const { getByTestId } = render(<ClientUpdatePrompt />);
    fireEvent.click(getByTestId("client-update-later"));
    expect(hook.dismiss).toHaveBeenCalledTimes(1);
    expect(updates.reloadAsync).not.toHaveBeenCalled();
  });

  it("stays silent when the downloaded bundle is the version already running", () => {
    updates.pending = true;
    updates.version = "1.0.0";
    const { container } = render(<ClientUpdatePrompt />);
    expect(container.innerHTML).toBe("");
  });

  it("never opens over the player", () => {
    updates.pending = true;
    route.pathname = "/player/42";
    const { container } = render(<ClientUpdatePrompt />);
    expect(container.innerHTML).toBe("");
  });
});
