import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  TextInput: ({ value, onChangeText, onSubmitEditing, placeholder, accessibilityLabel }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel ?? placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
      onKeyDown: (e: React.KeyboardEvent) => {
        if (e.key === "Enter") onSubmitEditing?.();
      },
    }),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "identify-loading", ...p }),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "identify-modal" }, children) : null),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      { "data-testid": "identify-results" },
      (data ?? []).map((item: any, i: number) =>
        React.createElement(React.Fragment, { key: keyExtractor(item, i) }, renderItem({ item })),
      ),
    ),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333", onAccent: "#fff", bg: "#000", divider: "#333" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 8, md: 12, lg: 16 },
  }),
}));

const eveyFetchMock = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  getMetadataServiceListMetadataOverridesQueryKey: (id: string) => [`/api/v1/metadata/${id}/overrides`],
  getLibraryServiceGetMediaItemQueryKey: (id: string) => [`/api/v1/media/${id}`],
}));

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { success: (...a: unknown[]) => toastSuccess(...a), error: (...a: unknown[]) => toastError(...a) },
}));

import { IdentifyMetadataSheet } from "./IdentifyMetadataSheet";

const RESULT_A = { tmdb_id: "9001", title: "Fictional Feature One", year: 2021, overview: "A test overview.", poster_url: undefined };
const RESULT_B = { imdb_id: "tt9000001", title: "Fictional Feature Two", year: 2019, overview: "Another overview.", poster_url: undefined };

const defaultProps = {
  visible: true,
  onClose: vi.fn(),
  mediaItemId: "mi-1",
  initialQuery: "Fictional",
  mediaType: "movie" as const,
  onApplied: vi.fn(),
};

function renderSheet(overrides: Partial<typeof defaultProps> = {}) {
  return render(<IdentifyMetadataSheet {...defaultProps} {...overrides} />);
}

describe("IdentifyMetadataSheet", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when not visible", () => {
    renderSheet({ visible: false });
    expect(screen.queryByTestId("identify-modal")).toBeNull();
  });

  it("pre-fills the search input with initialQuery", () => {
    renderSheet();
    expect((screen.getByLabelText("Search title") as HTMLInputElement).value).toBe("Fictional");
  });

  it("calls eveyFetch search endpoint with query and type on search", async () => {
    eveyFetchMock.mockResolvedValueOnce([RESULT_A]);
    renderSheet();

    fireEvent.click(screen.getByLabelText("Search"));

    await waitFor(() => {
      expect(eveyFetchMock).toHaveBeenCalledWith({
        url: "/api/v1/admin/media/mi-1/search-metadata",
        method: "GET",
        params: { query: "Fictional", type: "movie" },
      });
    });
  });

  it("renders results with title and year after a successful search", async () => {
    eveyFetchMock.mockResolvedValueOnce([RESULT_A, RESULT_B]);
    renderSheet();

    fireEvent.click(screen.getByLabelText("Search"));

    await waitFor(() => expect(screen.getByText(/Fictional Feature One \(2021\)/)).toBeTruthy());
    expect(screen.getByText(/Fictional Feature Two \(2019\)/)).toBeTruthy();
  });

  it("shows a no-results message when the search returns empty", async () => {
    eveyFetchMock.mockResolvedValueOnce([]);
    renderSheet();

    fireEvent.click(screen.getByLabelText("Search"));

    await waitFor(() => expect(screen.getByText("No TMDB matches. Try a shorter or differently spelled query.")).toBeTruthy());
  });

  it("applies a tmdb result via refresh-metadata and closes on success", async () => {
    eveyFetchMock.mockResolvedValueOnce([RESULT_A]).mockResolvedValueOnce(undefined);
    const onApplied = vi.fn();
    const onClose = vi.fn();
    renderSheet({ onApplied, onClose });

    fireEvent.click(screen.getByLabelText("Search"));
    await waitFor(() => screen.getByText(/Fictional Feature One/));

    fireEvent.click(screen.getByLabelText("Use this match"));

    await waitFor(() => {
      expect(eveyFetchMock).toHaveBeenCalledWith({
        url: "/api/v1/admin/media/mi-1/refresh-metadata",
        method: "POST",
        data: { tmdb_id: "9001", media_type: "movie" },
      });
    });
    await waitFor(() => expect(onApplied).toHaveBeenCalledOnce());
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  });

  it("toasts an error when apply fails", async () => {
    eveyFetchMock.mockResolvedValueOnce([RESULT_A]).mockRejectedValueOnce(new Error("boom"));
    renderSheet();

    fireEvent.click(screen.getByLabelText("Search"));
    await waitFor(() => screen.getByText(/Fictional Feature One/));
    fireEvent.click(screen.getByLabelText("Use this match"));

    await waitFor(() => expect(toastError).toHaveBeenCalledWith(expect.stringContaining("boom")));
  });
});
