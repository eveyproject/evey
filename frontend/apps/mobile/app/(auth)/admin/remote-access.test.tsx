import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The form is seeded from the server; Save appears in the header only once
// something changed and sends the LAN list split on commas and the bitrate as
// a number; turning remote access off clears the external URL; the server's
// own lockout warning shows before anything is saved; a refused save toasts.
const header = vi.hoisted(() => ({ right: null as null | (() => React.ReactNode) }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  RefreshControl: () => null,
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("input", { type: "checkbox", role: "switch", checked: value, onChange: (e: any) => onValueChange(e.target.checked) }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) => {
      header.right = options.headerRight ?? null;
      return options.headerRight ? React.createElement("div", { "data-testid": "header" }, options.headerRight()) : null;
    },
  },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useDemoStore: (sel: (s: any) => unknown) => sel({ demoMode: false, demoLocked: [] }), demoLocks: (s: any, k: string) => s.demoMode || s.demoLocked.includes(k) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", warning: "#fa0", success: "#0f0" }, radii: { md: 8 } }),
}));

import AdminRemoteAccessScreen from "./remote-access";

const config = { enabled: true, externalUrl: "https://evey.example", serverBaseUrl: "", lanNetworks: ["10.0.0.0/8", "192.168.1.0/24"], maxRemoteBitrateKbps: 8000 };

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminRemoteAccessScreen /></QueryClientProvider>);
}
const field = (label: string) => screen.getByLabelText(`admin.remoteAccess.${label}`) as HTMLInputElement;

beforeEach(() => {
  vi.clearAllMocks();
  header.right = null;
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.method === "PUT") return config;
    if (req.url.endsWith("network-info")) return { interfaces: [{ name: "eth0", address: "192.168.1.10" }] };
    return config;
  });
});

describe("admin remote access screen (mobile)", () => {
  it("seeds the form, shows Save only once dirty, and sends the LAN list split and the bitrate as a number", async () => {
    mount();
    await waitFor(() => expect(field("externalUrl").value).toBe("https://evey.example"));
    expect(field("lanNetworks").value).toBe("10.0.0.0/8, 192.168.1.0/24");
    expect(field("maxRemoteBitrate").value).toBe("8000");
    expect(screen.getByText("eth0")).toBeTruthy();
    expect(screen.getByText("192.168.1.10")).toBeTruthy();
    expect(screen.queryByTestId("header")).toBeNull();

    fireEvent.change(field("lanNetworks"), { target: { value: " 10.0.0.0/8 ,, 172.16.0.0/12 " } });
    fireEvent.change(field("maxRemoteBitrate"), { target: { value: "abc" } });
    expect(screen.getByTestId("header")).toBeTruthy();
    fireEvent.click(screen.getByText("common.save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.remoteAccess.saved"));
    expect(eveyFetch).toHaveBeenCalledWith({
      url: "/api/v1/admin/remote-access", method: "PUT",
      data: {
        enabled: true, externalUrl: "https://evey.example", lanNetworks: ["10.0.0.0/8", "172.16.0.0/12"], maxRemoteBitrateKbps: 0,
        trustedProxies: [], clearTrustedProxies: true,
      },
    });
    await waitFor(() => expect(screen.queryByTestId("header")).toBeNull());
  });

  it("sends the trusted proxy list split, and says where the current list comes from", async () => {
    eveyFetch.mockImplementation(async (req: { url: string }) =>
      req.url.endsWith("network-info") ? { interfaces: [] } : { ...config, trustedProxies: ["127.0.0.0/8"] },
    );
    mount();
    await waitFor(() => expect(field("trustedProxies").value).toBe("127.0.0.0/8"));
    expect(screen.getByText(/admin\.remoteAccess\.trustedProxiesFromEnv/)).toBeTruthy();
    fireEvent.change(field("trustedProxies"), { target: { value: "172.18.0.0/16, 10.0.0.2" } });
    fireEvent.click(screen.getByText("common.save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(eveyFetch).toHaveBeenCalledWith(expect.objectContaining({
      method: "PUT",
      data: expect.objectContaining({ trustedProxies: ["172.18.0.0/16", "10.0.0.2"], clearTrustedProxies: false }),
    }));
  });

  it("turning remote access off clears the external URL, and a refused save toasts", async () => {
    mount();
    await waitFor(() => expect(field("externalUrl").value).toBe("https://evey.example"));
    fireEvent.click(screen.getByRole("switch"));
    expect(field("externalUrl").value).toBe("");
    eveyFetch.mockImplementationOnce(async () => { throw new Error("403"); });
    fireEvent.click(screen.getByText("common.save"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("common.error"));
    expect(screen.getByTestId("header")).toBeTruthy();
  });

  it("warns about a lockout before anything is saved, and reports a published setup", async () => {
    eveyFetch.mockImplementation(async (req: { url: string }) =>
      req.url.endsWith("network-info") ? { interfaces: [] } : { ...config, lockoutRisk: true },
    );
    const first = mount();
    await waitFor(() => expect(screen.getByText("Lockout risk")).toBeTruthy());
    expect(screen.queryByText("admin.remoteAccess.networkInterfaces")).toBeNull();
    first.unmount();

    eveyFetch.mockImplementation(async (req: { url: string }) =>
      req.url.endsWith("network-info") ? { interfaces: [] } : { ...config, published: true },
    );
    mount();
    await waitFor(() => expect(screen.getByText("Remote access is published and reachable.")).toBeTruthy());
  });
});
