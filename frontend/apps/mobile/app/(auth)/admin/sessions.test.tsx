import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The live list polls every five seconds (never in the background); a kill
// is confirmed by name before the DELETE and the list refreshed after it; a
// remote command is a POST with the verb; an empty list and a failed fetch
// are two different screens (the failure has a retry).
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListEmptyComponent }: any) =>
    React.createElement("div", null, ListHeaderComponent, (data ?? []).length === 0
      ? ListEmptyComponent
      : (data as any[]).map((item) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  Alert: { alert: (title: string, message: string, buttons: any[]) => { alerts.last = { title, message, buttons }; } },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: string | Record<string, unknown>) => (typeof o === "string" ? o : o ? `${k}:${Object.values(o).join(",")}` : k) }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888" } }) }));
vi.mock("../../../src/components/admin/SessionCard", () => ({
  SessionCard: ({ session, onKill, onCommand }: any) =>
    React.createElement("div", { "data-testid": "session" },
      session.profileName,
      React.createElement("button", { onClick: () => onKill(session) }, `kill-${session.sessionId}`),
      React.createElement("button", { onClick: () => onCommand(session.sessionId, "pause") }, `pause-${session.sessionId}`)),
}));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import AdminSessionsScreen from "./sessions";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminSessionsScreen /></QueryClientProvider>);
}

const sessions = [{ sessionId: "s1", profileName: "Alice" }, { sessionId: "s2", profileName: "Kid" }];

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  eveyFetch.mockImplementation(async (req: { method: string }) => (req.method === "GET" ? { sessions } : {}));
});

describe("admin sessions screen (mobile)", () => {
  it("lists live sessions with the count and confirms a kill by name before deleting", async () => {
    mount();
    await waitFor(() => expect(screen.getAllByTestId("session")).toHaveLength(2));
    expect(screen.getByText("admin.sessions.activeCount:2")).toBeTruthy();
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/sessions/active", method: "GET" });

    fireEvent.click(screen.getByText("kill-s2"));
    expect(alerts.last.message).toBe("Stop Kid?");
    expect(eveyFetch).not.toHaveBeenCalledWith(expect.objectContaining({ method: "DELETE" }));
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Session stopped"));
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/sessions/s2", method: "DELETE" });
    await waitFor(() => expect(eveyFetch.mock.calls.filter((c) => (c[0] as { method: string }).method === "GET").length).toBeGreaterThanOrEqual(2));
  });

  it("sends a remote command as a POST and toasts a refusal", async () => {
    mount();
    await waitFor(() => expect(screen.getAllByTestId("session")).toHaveLength(2));
    fireEvent.click(screen.getByText("pause-s1"));
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledWith({
      url: "/api/v1/admin/sessions/s1/command", method: "POST", headers: { "Content-Type": "application/json" }, data: { command: "pause" },
    }));
    eveyFetch.mockImplementationOnce(async () => { throw new Error("gone"); });
    fireEvent.click(screen.getByText("pause-s2"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Failed to send command"));
  });

  it("keeps an empty list and a failed fetch apart", async () => {
    eveyFetch.mockResolvedValue({ sessions: [] });
    const first = mount();
    await waitFor(() => expect(screen.getByText("No active sessions")).toBeTruthy());
    expect(screen.queryByText("retry")).toBeNull();
    first.unmount();

    eveyFetch.mockRejectedValue(new Error("500"));
    mount();
    await waitFor(() => expect(screen.getByText("retry")).toBeTruthy());
    expect(screen.queryByText("No active sessions")).toBeNull();
  });
});
