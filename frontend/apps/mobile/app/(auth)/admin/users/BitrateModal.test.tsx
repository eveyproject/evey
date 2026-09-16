import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The cap is read only once the sheet is open (no fetch for a closed sheet),
// the current cap is ticked with "Unlimited" as 0, a pick patches the
// number, refreshes the read and closes; a refused patch is a toast and the
// sheet stays open.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: string | { defaultValue?: string }) => (typeof o === "string" ? o : o?.defaultValue ?? k) }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 14, lg: 18 } }));
vi.mock("../../../../src/components/BottomSheet", () => ({
  BottomSheet: ({ visible, title, children }: any) => (visible ? React.createElement("div", { "data-testid": "sheet" }, React.createElement("h2", null, title), children) : null),
}));
vi.mock("../../../../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { accent: "#f00", border: "#333", text: "#fff" } }) }));

import { BitrateModal } from "./BitrateModal";

const user = { id: "u1", username: "bob" } as any;

function mount(visible: boolean, onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><BitrateModal user={user} visible={visible} onClose={onClose} /></QueryClientProvider>);
  return onClose;
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetch.mockImplementation(async (req: { method: string }) => (req.method === "GET" ? { max_bitrate_kbps: 4000 } : undefined));
});

describe("BitrateModal (mobile)", () => {
  it("reads nothing while closed", () => {
    mount(false);
    expect(screen.queryByTestId("sheet")).toBeNull();
    expect(eveyFetch).not.toHaveBeenCalled();
  });

  it("ticks the current cap, patches the picked one, refreshes and closes", async () => {
    const onClose = mount(true);
    expect(screen.getByRole("heading").textContent).toBe("Max bitrate - bob");
    await waitFor(() => expect(screen.getByText("✓")).toBeTruthy());
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/users/u1/bitrate-limit", method: "GET" });
    expect(screen.getByRole("button", { name: "4 Mbps" }).textContent).toContain("✓");
    expect(screen.getByRole("button", { name: "Unlimited" }).textContent).not.toContain("✓");

    fireEvent.click(screen.getByRole("button", { name: "Unlimited" }));
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/users/u1/bitrate-limit", method: "PATCH", data: { max_bitrate_kbps: 0 } });
    expect(toast.success).toHaveBeenCalledWith("Bitrate limit updated");
    // The read is refreshed after the write.
    expect(eveyFetch.mock.calls.filter((c) => (c[0] as { method: string }).method === "GET").length).toBeGreaterThanOrEqual(2);
  });

  it("keeps the sheet open and toasts when the patch is refused", async () => {
    eveyFetch.mockImplementation(async (req: { method: string }) => {
      if (req.method === "GET") return { max_bitrate_kbps: 0 };
      throw new Error("403");
    });
    const onClose = mount(true);
    await waitFor(() => expect(screen.getByRole("button", { name: "Unlimited" }).textContent).toContain("✓"));
    fireEvent.click(screen.getByRole("button", { name: "20 Mbps" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Failed to update bitrate limit"));
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByTestId("sheet")).toBeTruthy();
  });
});
