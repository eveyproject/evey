import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress }: any) => React.createElement("button", { onClick: onPress }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: unknown) => {
      if (key === "admin.plugins.security.sandboxResolvedFrom") return `resolved from ${(opts as { mode: string }).mode}`;
      return typeof opts === "string" ? opts : key;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", success: "#0a0", warning: "#fa0", error: "#f33" },
    radii: { sm: 4, md: 8 },
  }),
}));
const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a) }));

import { PluginSecurityDashboard } from "./PluginSecurityDashboard";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(PluginSecurityDashboard)));
}

beforeEach(() => eveyFetchMock.mockReset());

// The dashboard exists so an admin learns the host cannot isolate plugins:
// a degraded sandbox names which probe failed, and each plugin reads by its
// worst finding, its signature, and its last error behind a fold.
describe("PluginSecurityDashboard", () => {
  it("shows the degraded sandbox with its failed probes even with no plugin", async () => {
    eveyFetchMock.mockResolvedValue({
      plugins: [],
      sandbox: { configured: "strict", effective: "basic", landlockKernel: true, launcherPresent: false, bubblewrapUsable: false },
    });
    mount();
    expect(await screen.findByText("basic")).toBeTruthy();
    expect(screen.getByText("resolved from strict")).toBeTruthy();
    expect(screen.getByText("Falling back to basic isolation.")).toBeTruthy();
    expect(screen.getByText("No plugin security data.")).toBeTruthy();
    const marks = screen.getAllByText(/[✓✗]/).map((e) => e.textContent);
    expect(marks).toEqual(["✓", "✗", "✗"]);
  });

  it("labels each plugin by its worst finding and unfolds the details", async () => {
    eveyFetchMock.mockResolvedValue({
      plugins: [
        {
          id: "p1", name: "Risky", version: "1.2.0", sandboxMode: "strict", permissions: ["subprocess"], sourceSHA256: "abcdef0123456789",
          findings: [
            { category: "dynamic-eval", severity: "critical", file: "main.js", line: 3, description: "eval on input" },
            { category: "overreach", severity: "warning", file: "a.js", line: 9 },
          ],
          signature: { verified: false },
          lastError: { ts: "2026-09-16T02:00:00Z", message: "socket timeout" },
          installedAt: "2026-09-15T00:00:00Z", sizeBytes: 10,
        },
        { id: "p2", name: "Tidy", version: "0.1.0", sandboxMode: "basic", permissions: [], findings: [], signature: { verified: true, verifiedBy: "issuer" }, sourceSHA256: "", installedAt: "", sizeBytes: 1 },
      ],
    });
    mount();
    expect(await screen.findByText("Risky")).toBeTruthy();
    expect(screen.getByText((_, el) => el?.tagName === "SPAN" && el.textContent === "v1.2.0  abcdef01")).toBeTruthy();
    expect(screen.getByText("1 critical")).toBeTruthy();
    expect(screen.getByText("unsigned")).toBeTruthy();
    expect(screen.getByText("subprocess")).toBeTruthy();
    expect(screen.getByText("error")).toBeTruthy();
    expect(screen.getByText("clean")).toBeTruthy();
    expect(screen.getByText("signed")).toBeTruthy();
    expect(screen.queryByText("Findings")).toBeNull();

    fireEvent.click(screen.getByText("Risky").closest("button")!);
    await waitFor(() => expect(screen.getByText("Findings")).toBeTruthy());
    expect(screen.getByText("CRITICAL")).toBeTruthy();
    expect(screen.getByText("dynamic-eval")).toBeTruthy();
    expect(screen.getByText("eval on input")).toBeTruthy();
    expect(screen.getByText("socket timeout")).toBeTruthy();

    // A clean plugin has nothing to unfold.
    fireEvent.click(screen.getByText("Tidy").closest("button")!);
    expect(screen.getAllByText("Findings")).toHaveLength(1);
  });
});
