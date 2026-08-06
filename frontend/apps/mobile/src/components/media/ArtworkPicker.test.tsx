import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333", onAccent: "#fff", overlay: "rgba(0,0,0,.5)" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 8, md: 12, lg: 16 },
  }),
}));

const eveyFetchMock = vi.fn();
let artworkQueryState: { data?: unknown };

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  useMetadataServiceGetArtwork: () => artworkQueryState,
  getMetadataServiceGetArtworkQueryKey: (id: string) => [`/api/v1/metadata/${id}/artwork`],
  getLibraryServiceGetMediaItemQueryKey: (id: string) => [`/api/v1/media/${id}`],
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { success: (...a: unknown[]) => toastSuccess(...a), error: (...a: unknown[]) => toastError(...a) },
}));

import { ArtworkPicker } from "./ArtworkPicker";

function renderPicker(mediaItemId = "m1") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    React.createElement(QueryClientProvider, { client: qc }, React.createElement(ArtworkPicker, { mediaItemId })),
  );
}

const POSTER_A = { id: "art-1", type: "ARTWORK_TYPE_PRIMARY", url: "https://img.example/p1.jpg" };
const POSTER_B = { id: "art-2", type: "ARTWORK_TYPE_PRIMARY", url: "https://img.example/p2.jpg" };
const BACKDROP_A = { id: "art-3", type: "ARTWORK_TYPE_BACKDROP", url: "https://img.example/b1.jpg" };

describe("ArtworkPicker", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    artworkQueryState = { data: undefined };
  });

  it("renders nothing when there is only one poster candidate and no backdrop choice", () => {
    artworkQueryState = { data: { artwork: [POSTER_A] } };
    const { container } = renderPicker();
    expect(container.textContent).toBe("");
  });

  it("shows selectable poster thumbnails once there are multiple candidates", () => {
    artworkQueryState = { data: { artwork: [POSTER_A, POSTER_B] } };
    renderPicker();
    expect(screen.getByLabelText("media.detail.poster 1")).toBeTruthy();
    expect(screen.getByLabelText("media.detail.poster 2")).toBeTruthy();
  });

  it("shows selectable backdrop thumbnails once there are multiple candidates", () => {
    artworkQueryState = { data: { artwork: [BACKDROP_A, { ...BACKDROP_A, id: "art-4" }] } };
    renderPicker();
    expect(screen.getByLabelText("media.detail.backdrop 1")).toBeTruthy();
    expect(screen.getByLabelText("media.detail.backdrop 2")).toBeTruthy();
  });

  it("selects an artwork by posting to the select endpoint and toasts success", async () => {
    artworkQueryState = { data: { artwork: [POSTER_A, POSTER_B] } };
    eveyFetchMock.mockResolvedValueOnce(undefined);
    renderPicker("m1");

    fireEvent.click(screen.getByLabelText("media.detail.poster 2"));

    await waitFor(() => {
      expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/media/m1/artwork/art-2/select", method: "POST" });
    });
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith("media.detail.artworkUpdated"));
  });

  it("toasts an error when selection fails", async () => {
    artworkQueryState = { data: { artwork: [POSTER_A, POSTER_B] } };
    eveyFetchMock.mockRejectedValueOnce(new Error("network down"));
    renderPicker("m1");

    fireEvent.click(screen.getByLabelText("media.detail.poster 2"));

    await waitFor(() => expect(toastError).toHaveBeenCalledWith("media.detail.artworkFailed"));
  });
});
