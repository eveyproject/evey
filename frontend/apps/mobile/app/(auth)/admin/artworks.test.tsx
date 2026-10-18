import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Each artwork action toasts its own failure. The app-wide mutation net only
// sees a hook-level onError, so without meta.silentError it toasted a second,
// generic "Error" on top of the screen's message.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: string | Record<string, unknown>) => (typeof o === "string" ? o : typeof o?.defaultValue === "string" ? o.defaultValue : k),
  }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: new Proxy({}, { get: () => "#888" }),
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: new Proxy({}, { get: () => 8 }),
  }),
}));

import { mutationErrorNet } from "../../../src/lib/mutationErrorNet";
import ArtworksAdminScreen from "./artworks";

function mount() {
  // The app's real net, so a second toast from it lands on the same mock.
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    mutationCache: mutationErrorNet(),
  });
  return render(<QueryClientProvider client={qc}><ArtworksAdminScreen /></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/artworks/stats") return { totalRecords: 10, withLocalPath: 5, missingLocalPath: 5, totalSizeBytes: 0 };
    throw new Error("500");
  });
});

describe("admin artworks screen (mobile)", () => {
  it.each([
    ["Download missing", "Download failed"],
    ["Lancer le backfill", "Échec du backfill des blurhash"],
    ["Purger", "Échec de la purge des variantes"],
  ])("a failed %s shows one toast, the screen's own", async (label, message) => {
    mount();
    await screen.findByText(/cached locally/);
    fireEvent.click(screen.getByRole("button", { name: label }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(message));
    // Let the net run too before counting.
    await new Promise((r) => setTimeout(r, 0));
    expect(toast.error).toHaveBeenCalledTimes(1);
  });
});
