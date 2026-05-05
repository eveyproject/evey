import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  ActivityIndicator: (p: any) =>
    React.createElement("div", { "data-testid": "loading", role: "progressbar", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Platform: { OS: "android", select: (o: any) => o.android },
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- expo-router ---------------------------------------------------------------------
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) =>
      React.createElement("div", { "data-testid": "stack-screen", title: options?.title }),
  },
  useRouter: () => ({ replace: vi.fn(), back: vi.fn(), push: vi.fn() }),
}));

// --- ThemeProvider -------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      surfaceRaised: "#222",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      border: "#333",
      error: "#e00",
      warning: "#fa0",
      success: "#0f0",
    },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

// --- eveyFetch mock ------------------------------------------------------------------
const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

// --- toast mock ----------------------------------------------------------------------
// vi.hoisted ensures these refs are available inside the vi.mock factory (which is hoisted)
const { toastSuccessMock, toastErrorMock } = vi.hoisted(() => ({
  toastSuccessMock: vi.fn(),
  toastErrorMock: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  toast: { success: toastSuccessMock, error: toastErrorMock },
}));

// -------------------------------------------------------------------------------------
import ArtworksAdminScreen from "../../app/(auth)/admin/artworks";

const STATS_ZERO = { totalRecords: 0, withLocalPath: 0, missingLocalPath: 0, totalSizeBytes: 0 };
const STATS_PARTIAL = { totalRecords: 10, withLocalPath: 3, missingLocalPath: 7, totalSizeBytes: 0 };
const STATS_DONE = { totalRecords: 10, withLocalPath: 10, missingLocalPath: 0, totalSizeBytes: 1024 };
const STATS_HALF = { totalRecords: 10, withLocalPath: 5, missingLocalPath: 5, totalSizeBytes: 0 };

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

// Route eveyFetch calls by URL
function routeFetch(
  statsData: object,
  jobId: string | null = null,
  jobStatus: object | null = null,
) {
  eveyFetchMock.mockImplementation(({ url, method }: { url: string; method: string }) => {
    if (url === "/api/v1/admin/artworks/stats") return Promise.resolve(statsData);
    if (url === "/api/v1/admin/artworks/download-all" && method === "POST") {
      return Promise.resolve({ jobId: jobId ?? "job-x", totalToDownload: 5 });
    }
    if (jobId && url === `/api/v1/admin/artworks/download-all/${jobId}`) {
      return Promise.resolve(jobStatus ?? { status: "running", processed: 0, total: 5, errors: [] });
    }
    return Promise.reject(new Error(`Unexpected fetch: ${method} ${url}`));
  });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("ArtworksAdminScreen", () => {
  it("renders the cache status card and Download missing button in initial state", async () => {
    routeFetch(STATS_ZERO);

    render(<ArtworksAdminScreen />, { wrapper: makeWrapper() });

    // Cache status card title is always present
    expect(screen.getByText("Cache status")).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Download missing" })).toBeTruthy();
    });
  });

  it("shows ActivityIndicator while stats are loading", () => {
    // Stats never resolve → perpetual loading
    eveyFetchMock.mockReturnValue(new Promise(() => {}));

    render(<ArtworksAdminScreen />, { wrapper: makeWrapper() });

    expect(screen.getByTestId("loading")).toBeTruthy();
  });

  it("shows 50% when withLocalPath:5 and totalRecords:10", async () => {
    routeFetch(STATS_HALF);

    render(<ArtworksAdminScreen />, { wrapper: makeWrapper() });

    await waitFor(() => {
      expect(screen.getByText("50%")).toBeTruthy();
    });
  });

  it("calls POST /api/v1/admin/artworks/download-all on Download missing tap", async () => {
    routeFetch(STATS_PARTIAL, "job-1", { status: "running", processed: 0, total: 7, errors: [] });

    render(<ArtworksAdminScreen />, { wrapper: makeWrapper() });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Download missing" })).toBeTruthy();
    });

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Download missing" }));
    });

    await waitFor(() => {
      const postCall = eveyFetchMock.mock.calls.find(
        (call: any[]) =>
          call[0]?.url === "/api/v1/admin/artworks/download-all" &&
          call[0]?.method === "POST",
      );
      expect(postCall).toBeTruthy();
    });
  });

  it("calls toast.success once when polled job status returns done", async () => {
    const JOB_ID = "job-done-1";
    let statsCallCount = 0;

    eveyFetchMock.mockImplementation(({ url, method }: { url: string; method: string }) => {
      if (url === "/api/v1/admin/artworks/stats") {
        statsCallCount += 1;
        // Return partial stats on first call, full stats after job completes
        return Promise.resolve(statsCallCount === 1 ? STATS_PARTIAL : STATS_DONE);
      }
      if (url === "/api/v1/admin/artworks/download-all" && method === "POST") {
        return Promise.resolve({ jobId: JOB_ID, totalToDownload: 7 });
      }
      if (url === `/api/v1/admin/artworks/download-all/${JOB_ID}`) {
        return Promise.resolve({ status: "done", processed: 7, total: 7, errors: [] });
      }
      return Promise.reject(new Error(`Unexpected fetch: ${method} ${url}`));
    });

    render(<ArtworksAdminScreen />, { wrapper: makeWrapper() });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Download missing" })).toBeTruthy();
    });

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Download missing" }));
    });

    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledTimes(1), { timeout: 5000 });
    expect(toastSuccessMock).toHaveBeenCalledWith("Artwork download complete");
  });
});
