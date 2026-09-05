import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";

// --- React Native mock ----------------------------------------------------------------
// The FlatList mock exposes onEndReached as a button: paging is the whole
// subject of this file, so the test has to be able to ask for the next page.
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Share: { share: vi.fn() },
  FlatList: ({ data, renderItem, keyExtractor, onEndReached, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "list" },
      data.length === 0
        ? (typeof ListEmptyComponent === "function" ? React.createElement(ListEmptyComponent) : ListEmptyComponent)
        : data.map((item: any, i: number) =>
            React.createElement(React.Fragment, { key: keyExtractor(item, i) }, renderItem({ item })),
          ),
      React.createElement("button", { onClick: () => onEndReached?.(), "aria-label": "load-more" }, "more"),
    ),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// searchParams: the screen seeds its watcher from a ?profileId= deep link
// (the Users screen's per-profile "History" button). Empty here - these tests
// cover the unfiltered landing.
let searchParams: Record<string, string> = {};
vi.mock("expo-router", () => ({
  Stack: { Screen: () => React.createElement("div", { "data-testid": "stack-screen" }) },
  useLocalSearchParams: () => searchParams,
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

vi.mock("../components/LoadFailed", () => ({
  LoadFailed: () => React.createElement("div", { "data-testid": "load-failed" }),
}));

vi.mock("@evey/store", () => ({ toast: { error: vi.fn(), info: vi.fn() } }));

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: undefined }),
}));

const playHistoryMock = vi.fn();
vi.mock("@evey/api", async () => {
  // The screen renders through the real formatters - stubbing them here would
  // hide exactly the drift the shared module exists to prevent.
  const { formatBytes, formatDurationMs } = await vi.importActual<typeof import("@evey/api")>("@evey/api");
  return {
    formatBytes,
    formatDurationMs,
    useActivityServiceGetPlayHistory: (...a: unknown[]) => playHistoryMock(...a),
    useActivityServiceGetServerStats: () => ({ data: undefined, isLoading: false, refetch: vi.fn() }),
    useAdminSessionServiceGetAuditLog: () => ({ data: undefined, isLoading: false, isError: false, refetch: vi.fn() }),
    activityServiceGetPlayHistory: vi.fn(),
    activityServiceGetServerStats: vi.fn(),
    adminSessionServiceGetAuditLog: vi.fn(),
    // Read by the watcher filter, which labels each chip with its owner.
    useUserServiceListUsers: () => ({ data: { users: [] } }),
    useUserServiceListAllProfiles: () => ({ data: { profiles: [] } }),
    collectAllPages: vi.fn(),
    apiErrorMessage: (_e: unknown, _t: unknown, fallback: string) => fallback,
    EXPORT_PAGE_SIZE: 500,
    EXPORT_MAX_ROWS: 10000,
    eveyFetch: vi.fn(),
  };
});

import ActivityScreen from "../../app/(auth)/admin/activity";

function page(n: number, size = 25) {
  return Array.from({ length: size }, (_, i) => ({
    id: `p${n}-${i}`,
    mediaTitle: `Title ${n}-${i}`,
    durationMs: "1000",
    positionMs: "500",
    startedAt: new Date().toISOString(),
  }));
}

const refetch = vi.fn();
// The hook answers the page it was ASKED for. A mock that ignores the argument
// re-serves page 1 under page 2 and the screen appends it twice - the test then
// measures the mock, not the screen.
let failPage = 0;
// One STABLE object per page, like react-query: a mock that rebuilds its result
// on every render changes `data`'s identity each time, which is not something
// the real hook ever does. The first version of this file did, and the screen
// re-ran its append effect for ever.
let served: Map<number, unknown>;

beforeEach(() => {
  vi.clearAllMocks();
  searchParams = {};
  failPage = 0;
  served = new Map();
  playHistoryMock.mockImplementation((params: any) => {
    const p = params?.["pagination.page"] ?? 1;
    if (p === failPage) {
      return { data: undefined, isLoading: false, isError: true, refetch };
    }
    if (!served.has(p)) {
      served.set(p, { entries: page(p), pagination: { totalItems: 50 } });
    }
    return { data: served.get(p), isLoading: false, isError: false, refetch };
  });
});

describe("admin activity - play history paging", () => {
  it("renders the first page", async () => {
    render(<ActivityScreen />);
    await waitFor(() => expect(screen.getByText("Title 1-0")).toBeTruthy());
  });

  // The defect: entries were read straight off `data`, which is undefined once
  // the query errors, so a failed SECOND page wiped everything already read and
  // the screen fell back to its empty state - as if there were no history.
  it("keeps the pages already read when the next one fails", async () => {
    failPage = 2;
    render(<ActivityScreen />);
    await waitFor(() => expect(screen.getByText("Title 1-0")).toBeTruthy());

    await act(async () => {
      fireEvent.click(screen.getByLabelText("load-more"));
    });

    expect(screen.getByText("Title 1-0")).toBeTruthy();
    expect(screen.getByText("Title 1-24")).toBeTruthy();
    expect(screen.queryByTestId("load-failed")).toBeNull();
  });

  it("appends the next page instead of replacing the first", async () => {
    render(<ActivityScreen />);
    await waitFor(() => expect(screen.getByText("Title 1-0")).toBeTruthy());

    await act(async () => {
      fireEvent.click(screen.getByLabelText("load-more"));
    });

    await waitFor(() => expect(screen.getByText("Title 2-0")).toBeTruthy());
    expect(screen.getByText("Title 1-0")).toBeTruthy();
  });

  // A first page that fails has nothing to preserve, and must say so rather
  // than show an empty list.
  it("still reports a failure when nothing was ever loaded", async () => {
    failPage = 1;
    render(<ActivityScreen />);
    await waitFor(() => expect(screen.getByTestId("load-failed")).toBeTruthy());
  });
});

// The Users screen's per-profile "History" button deep-links here with the
// profile attached, mirroring the web's /admin/activity?profileId=. Landing
// unfiltered would make the admin pick the same profile over again, which is
// the work the button exists to remove.
describe("admin activity - deep link from the users screen", () => {
  it("asks for the linked profile's history, not the whole server's", async () => {
    searchParams = { profileId: "prf-42" };
    render(<ActivityScreen />);

    await waitFor(() => expect(playHistoryMock).toHaveBeenCalled());
    expect(playHistoryMock.mock.calls[0][0]).toMatchObject({ profileId: "prf-42" });
  });

  it("asks for the whole server's history with no link", async () => {
    render(<ActivityScreen />);

    await waitFor(() => expect(playHistoryMock).toHaveBeenCalled());
    expect(playHistoryMock.mock.calls[0][0]).not.toHaveProperty("profileId");
  });
});
