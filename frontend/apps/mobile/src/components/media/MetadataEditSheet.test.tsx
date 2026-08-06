import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import type { V1MediaItem } from "@evey/api";

// This suite covers only the wiring added on top of the pre-existing field
// editor (rematch confirm, identify sheet, artwork picker): each new capability
// gets its own dedicated component test, so here we just verify MetadataEditSheet
// renders them and threads the right props/callbacks through.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, placeholder, accessibilityLabel }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel ?? placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Linking: { openURL: vi.fn() },
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222",
      border: "#333", onAccent: "#fff", bg: "#000", error: "#e00", warning: "#fa0", danger: "#e00", divider: "#333",
    },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 8, md: 12, lg: 16 },
  }),
}));

vi.mock("../BottomSheet", () => ({
  BottomSheet: ({ visible, title, children }: any) =>
    visible ? React.createElement("div", { "data-testid": "sheet" }, React.createElement("span", null, title), children) : null,
}));

const artworkPickerSpy = vi.fn();
vi.mock("./ArtworkPicker", () => ({
  ArtworkPicker: (props: any) => {
    artworkPickerSpy(props);
    return React.createElement("div", { "data-testid": "artwork-picker" });
  },
}));

const rematchModalSpy = vi.fn();
vi.mock("./RematchConfirmModal", () => ({
  RematchConfirmModal: (props: any) => {
    rematchModalSpy(props);
    return props.visible ? React.createElement("div", { "data-testid": "rematch-modal-mock" }) : null;
  },
}));

const identifySheetSpy = vi.fn();
vi.mock("./IdentifyMetadataSheet", () => ({
  IdentifyMetadataSheet: (props: any) => {
    identifySheetSpy(props);
    return props.visible ? React.createElement("div", { "data-testid": "identify-sheet-mock" }) : null;
  },
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

vi.mock("@evey/api", async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>();
  return {
    ...actual,
    useMetadataServiceSetMetadataOverride: () => ({ mutateAsync: vi.fn() }),
    useMetadataServiceRemoveMetadataOverride: () => ({ mutateAsync: vi.fn() }),
    useMetadataServiceRefreshMetadata: () => ({ mutateAsync: vi.fn() }),
    useMetadataServiceListMetadataOverrides: () => ({ data: { overrides: [] } }),
    getMetadataServiceListMetadataOverridesQueryKey: (id: string) => [`/api/v1/metadata/${id}/overrides`],
    getLibraryServiceGetMediaItemQueryKey: (id: string) => [`/api/v1/media/${id}`],
    eveyFetch: vi.fn(),
  };
});

import { MetadataEditSheet } from "./MetadataEditSheet";

const ITEM: V1MediaItem = {
  id: "mi-1",
  title: "Fictional Feature",
  type: "MEDIA_ITEM_TYPE_MOVIE" as V1MediaItem["type"],
};

describe("MetadataEditSheet - rematch, identify, artwork wiring", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the ArtworkPicker for the current item", () => {
    render(<MetadataEditSheet visible item={ITEM} onClose={vi.fn()} />);
    expect(screen.getByTestId("artwork-picker")).toBeTruthy();
    expect(artworkPickerSpy).toHaveBeenCalledWith(expect.objectContaining({ mediaItemId: "mi-1" }));
  });

  it("opens the rematch confirm modal when the rematch button is pressed", () => {
    render(<MetadataEditSheet visible item={ITEM} onClose={vi.fn()} />);
    expect(screen.queryByTestId("rematch-modal-mock")).toBeNull();

    fireEvent.click(screen.getByLabelText("media.detail.rematchMetadata"));

    expect(screen.getByTestId("rematch-modal-mock")).toBeTruthy();
  });

  it("opens the identify sheet, pre-filled with the item title, when the identify button is pressed", () => {
    render(<MetadataEditSheet visible item={ITEM} onClose={vi.fn()} />);

    fireEvent.click(screen.getByLabelText("Identify on TMDB"));

    expect(screen.getByTestId("identify-sheet-mock")).toBeTruthy();
    expect(identifySheetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ mediaItemId: "mi-1", initialQuery: "Fictional Feature", mediaType: "movie" }),
    );
  });
});
