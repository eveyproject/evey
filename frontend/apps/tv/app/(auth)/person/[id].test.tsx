import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The filmography is fetched whole (the heading's count must be the real
// total) but RENDERED 24 posters at a time behind "Show more", so a
// prolific actor no longer freezes the box; a failed person or filmography
// fetch is a retry rather than an "Unknown" header over an empty grid.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: ({ source }: any) => React.createElement("img", { src: source?.uri, alt: "" }) }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router, useLocalSearchParams: () => ({ id: "p1" }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const api = vi.hoisted(() => ({
  fetch: vi.fn(),
  media: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  mediaArgs: null as any,
}));
vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => api.fetch(...a),
  useLibraryServiceListMediaItems: (p: unknown) => {
    api.mediaArgs = p;
    return api.media;
  },
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" }, radii: { md: 8, sm: 4 } }),
}));
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "https://s" }));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({ Authorization: "Bearer t" }) }));
vi.mock("../../../src/lib/artwork", () => ({
  artworkUrl: (s: string, id: string, kind: string, w: number) => `${s}/art/${id}/${kind}/${w}`,
  artworkPx: (dp: number) => dp * 2,
}));
vi.mock("../../../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
}));
vi.mock("../../../src/components/ScreenState", () => ({
  ScreenState: ({ loading, error, onRetry }: any) =>
    React.createElement("div", { "data-testid": "state", "data-loading": String(loading), "data-error": String(error) }, React.createElement("button", { onClick: onRetry }, "retry")),
}));

import PersonDetailScreen from "./[id]";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><PersonDetailScreen /></QueryClientProvider>);
}
const items = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `m${i}`, title: `Film ${i}` }));

beforeEach(() => {
  vi.clearAllMocks();
  api.fetch.mockReset().mockResolvedValue({ id: "p1", name: "Jane Doe", biography: "A long life in pictures." });
  api.media = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("tv person screen", () => {
  it("counts the whole filmography but renders 24 posters at a time, opening one on press", async () => {
    api.media.data = { items: items(30) };
    mount();
    await waitFor(() => expect(screen.getByText("Jane Doe")).toBeTruthy());
    expect(api.mediaArgs).toEqual({ personId: "p1", "pagination.pageSize": 500 });
    expect(api.fetch).toHaveBeenCalledWith({ url: "/api/v1/people/p1", method: "GET" });
    expect(screen.getByText("media.person.filmography (30)")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /^Film / })).toHaveLength(24);
    fireEvent.click(screen.getByRole("button", { name: "Show more" }));
    expect(screen.getAllByRole("button", { name: /^Film / })).toHaveLength(30);
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Film 3" }));
    expect(router.push).toHaveBeenCalledWith("/(auth)/media/m3");
    expect((document.querySelector("img") as HTMLImageElement).getAttribute("src")).toContain("/art/m0/poster/");
  });

  it("expands the biography on demand", async () => {
    api.media.data = { items: [] };
    mount();
    await waitFor(() => expect(screen.getByRole("button", { name: "common.readMore" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "common.readMore" }));
    expect(screen.getByRole("button", { name: "common.showLess" })).toBeTruthy();
  });

  it("a failed fetch on either half is a retry that re-asks only the failed half", async () => {
    api.media.isError = true;
    api.media.data = undefined;
    mount();
    await waitFor(() => expect(screen.getByTestId("state").getAttribute("data-error")).toBe("true"));
    expect(screen.queryByText("common.unknown")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(api.media.refetch).toHaveBeenCalledTimes(1);
    expect(api.fetch).toHaveBeenCalledTimes(1);
  });
});
