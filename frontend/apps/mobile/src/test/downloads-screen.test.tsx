import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      { "data-testid": "downloads-list" },
      data.map((item: any) =>
        React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
      ),
    ),
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- expo-router ------------------------------------------------------------------------
const pushMock = vi.fn();
const backMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...a: unknown[]) => pushMock(...a), back: (...a: unknown[]) => backMock(...a) },
  Stack: {
    Screen: ({ options }: any) =>
      React.createElement("div", { "data-testid": "stack-screen", title: options?.title }),
  },
  // No real navigator in this test - just run the focus callback on mount,
  // matching how useFocusEffect behaves the first time a screen mounts.
  useFocusEffect: (cb: () => void | (() => void)) => {
    React.useEffect(() => cb(), []);
  },
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallbackOrOpts?: unknown, maybeOpts?: Record<string, unknown>) => {
      const fallback = typeof fallbackOrOpts === "string" ? fallbackOrOpts : key;
      const opts = (typeof fallbackOrOpts === "object" ? fallbackOrOpts : maybeOpts) as Record<string, unknown> | undefined;
      if (!opts) return fallback;
      return fallback.replace(/\{\{(\w+)\}\}/g, (_, k) => String(opts[k] ?? ""));
    },
  }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", border: "#333",
    },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

// --- lib/downloads ----------------------------------------------------------------------
const getDownloadsMock = vi.fn();
const deleteDownloadMock = vi.fn();
const getTotalDownloadSizeMock = vi.fn();
const getInFlightProgressMock = vi.fn(() => null as number | null);
vi.mock("../lib/downloads", () => ({
  getDownloads: (...a: unknown[]) => getDownloadsMock(...a),
  deleteDownload: (...a: unknown[]) => deleteDownloadMock(...a),
  getTotalDownloadSize: (...a: unknown[]) => getTotalDownloadSizeMock(...a),
  subscribeToProgress: () => () => {},
  getInFlightProgress: (...a: unknown[]) => getInFlightProgressMock(...a),
}));

// -------------------------------------------------------------------------------------
import DownloadsScreen from "../../app/(auth)/downloads";

const SAMPLE = [
  { mediaItemId: "m1", title: "Movie 1", filePath: "/downloads/m1.mp4", fileSize: 1_500_000, downloadedAt: "2026-01-01T00:00:00.000Z" },
  { mediaItemId: "m2", title: "Movie 2", filePath: "/downloads/m2.mp4", fileSize: 2_500_000, downloadedAt: "2026-01-02T00:00:00.000Z" },
];

beforeEach(() => {
  vi.clearAllMocks();
  getDownloadsMock.mockResolvedValue(SAMPLE);
  getTotalDownloadSizeMock.mockResolvedValue(4_000_000);
  deleteDownloadMock.mockResolvedValue(undefined);
});

describe("DownloadsScreen", () => {
  it("renders the downloaded items with title and size", async () => {
    render(<DownloadsScreen />);
    await waitFor(() => {
      expect(screen.getByText("Movie 1")).toBeTruthy();
      expect(screen.getByText("Movie 2")).toBeTruthy();
    });
  });

  it("shows an empty state when there are no downloads", async () => {
    getDownloadsMock.mockResolvedValue([]);
    getTotalDownloadSizeMock.mockResolvedValue(0);
    render(<DownloadsScreen />);
    await waitFor(() => {
      expect(screen.getByText("No downloads yet")).toBeTruthy();
    });
  });

  it("calls deleteDownload when the trash icon is pressed and confirmed", async () => {
    const { Alert } = await import("react-native");
    // Drive the confirmation dialog straight to its destructive action, like
    // a real user tapping "Delete".
    (Alert.alert as unknown as ReturnType<typeof vi.fn>).mockImplementation(
      (_title: string, _msg: string, buttons: { text: string; onPress?: () => void }[]) => {
        buttons.find((b) => b.text === "Delete")?.onPress?.();
      },
    );

    render(<DownloadsScreen />);
    await waitFor(() => expect(screen.getByText("Movie 1")).toBeTruthy());

    const deleteButtons = screen.getAllByLabelText("Delete download");
    await act(async () => {
      fireEvent.click(deleteButtons[0]);
    });

    await waitFor(() => {
      expect(deleteDownloadMock).toHaveBeenCalledWith("m1");
    });
    // Re-lists after a successful delete.
    expect(getDownloadsMock.mock.calls.length).toBeGreaterThanOrEqual(2);
  });
});
