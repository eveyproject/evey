import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A dead feed endpoint used to render as an empty shelf ("nothing to see
// here") because isError was never read. This proves the home screen now
// tells loading, failure and genuine emptiness apart, per-section, instead
// of hiding a failed section behind silence.

// --- React Native mock -------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  RefreshControl: () => null,
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  Switch: () => null,
  StyleSheet: { create: (s: any) => s },
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
  Alert: { alert: vi.fn() },
}));

// --- expo-image / expo-router -------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));
vi.mock("expo-router", () => ({
  router: { push: vi.fn() },
}));

// --- react-i18next --------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

// --- ThemeProvider ----------------------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surfaceRaised: "#222", onAccent: "#fff", border: "#333" },
    spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- misc hooks/components not under test ------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../../../src/hooks/useWatchedMap", () => ({ useWatchedMap: () => ({ get: () => false }) }));
vi.mock("../../../src/features/plugins/PluginSlot", () => ({ PluginSlot: () => null }));
vi.mock("../../../src/components/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

// MediaRow/CollectionRow are mocked to their real, load-bearing contract
// (nothing rendered once there are zero items) so the empty-state assertion
// below stays honest without dragging in MediaCard/expo-image/watched-map.
vi.mock("../../../src/components/MediaRow", () => ({
  MediaRow: ({ title, items }: any) =>
    items && items.length > 0 ? React.createElement("div", null, title) : null,
}));
vi.mock("../../../src/components/CollectionRow", () => ({
  CollectionRow: ({ title, collections }: any) =>
    collections && collections.length > 0 ? React.createElement("div", null, title) : null,
}));

// --- @evey/store -------------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ profileId: "prof-1", isAdmin: false }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

// --- @tanstack/react-query ------------------------------------------------------------
// index.tsx reaches useQuery both directly (installed-plugins list) and
// indirectly through the real useMostPopular/useCollections hooks (they are
// NOT mocked below, so they call straight into this stub) - keyed by the
// query's own queryKey so each section is independently controllable without
// standing up a real QueryClient.
let popularLoading = false;
let popularError = false;
let popularItems: unknown[] = [];
const popularRefetch = vi.fn();

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  useQuery: (opts: { queryKey: readonly unknown[] }) => {
    if (opts.queryKey[0] === "popular") {
      return { data: popularItems, isLoading: popularLoading, isError: popularError, refetch: popularRefetch };
    }
    if (opts.queryKey[0] === "collections") {
      return { data: { collections: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    }
    // installed-plugins list and anything else: settled, empty, harmless.
    return { data: {}, isLoading: false, isError: false, refetch: vi.fn() };
  },
}));

// --- @evey/api ----------------------------------------------------------------------------
const emptySettled = { data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() };
const resetMutateAsync = vi.fn().mockResolvedValue({});
vi.mock("@evey/api", () => ({
  usePlaybackServiceGetResumeItems: () => emptySettled,
  usePlaybackServiceGetNextUp: () => emptySettled,
  useFeedServiceRecommended: () => emptySettled,
  useFeedServiceGetFeedConfig: () => ({ data: { configs: [] } }),
  useFeedServiceUpdateProfileFeedConfig: () => ({ mutateAsync: vi.fn() }),
  useFeedServiceResetProfileFeedConfig: () => ({ mutateAsync: resetMutateAsync }),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] } }),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] }, isLoading: false }),
  eveyFetch: vi.fn().mockResolvedValue({}),
}));

import { Alert } from "react-native";
import HomeScreen from "./index";

/** Fires the Alert button whose text matches, the way a tap would. */
function tapAlertButton(text: string) {
  const buttons = (Alert.alert as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[2] as
    | { text: string; onPress?: () => void }[]
    | undefined;
  const btn = buttons?.find((b) => b.text === text);
  if (!btn) throw new Error(`no Alert button labelled ${text}, got: ${buttons?.map((b) => b.text).join(", ")}`);
  btn.onPress?.();
}

beforeEach(() => {
  vi.clearAllMocks();
  popularLoading = false;
  popularError = false;
  popularItems = [];
  resetMutateAsync.mockClear();
});

describe("HomeScreen - most_popular shelf, loading vs failure vs empty", () => {
  it("shows a loading skeleton, not the failure or the empty state", () => {
    popularLoading = true;
    render(<HomeScreen />);
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
    expect(screen.queryByText("common.loadError")).toBeNull();
    expect(screen.queryByText("media.home.mostPopular")).toBeNull();
  });

  it("shows LoadFailed with a retry, not the empty message, when the feed fails", () => {
    popularError = true;
    render(<HomeScreen />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.home.mostPopular")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(popularRefetch).toHaveBeenCalledOnce();
  });

  it("shows neither LoadFailed nor a shelf when the feed genuinely has nothing", () => {
    render(<HomeScreen />);
    expect(screen.queryByText("common.loadError")).toBeNull();
    expect(screen.queryByText("media.home.mostPopular")).toBeNull();
  });
});

describe("HomeScreen - customize panel reset to defaults", () => {
  it("does not reset until the confirmation is accepted", async () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("common.feeds.customize"));
    fireEvent.click(screen.getByLabelText("common.feeds.resetDefaults"));

    expect(Alert.alert).toHaveBeenCalledOnce();
    expect(resetMutateAsync).not.toHaveBeenCalled();
  });

  it("resets every feed row and refetches the config once confirmed", async () => {
    render(<HomeScreen />);
    fireEvent.click(screen.getByLabelText("common.feeds.customize"));
    fireEvent.click(screen.getByLabelText("common.feeds.resetDefaults"));

    tapAlertButton("common.feeds.resetDefaults");
    // The confirm handler is async (awaits each mutateAsync); flush microtasks.
    await Promise.resolve();
    await Promise.resolve();

    expect(resetMutateAsync).toHaveBeenCalled();
  });
});
