import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import React from "react";

// --- expo-image -------------------------------------------------------------------------
// The real module reaches for the native EventEmitter at import time; the row
// renders a cached poster through it.
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

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
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "activity-indicator", ...p }),
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
      textMuted: "#888", accent: "#f00", border: "#333", warning: "#f90",
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
const getInFlightDownloadsMock = vi.fn(() => [] as { mediaItemId: string; title: string }[]);
vi.mock("../lib/downloads", () => ({
  getDownloads: (...a: unknown[]) => getDownloadsMock(...a),
  deleteDownload: (...a: unknown[]) => deleteDownloadMock(...a),
  getTotalDownloadSize: (...a: unknown[]) => getTotalDownloadSizeMock(...a),
  subscribeToProgress: () => () => {},
  getInFlightProgress: (...a: unknown[]) => getInFlightProgressMock(...a),
  getInFlightBytes: () => null,
  getInFlightDownloads: (...a: unknown[]) => getInFlightDownloadsMock(...a),
  isPreparingCompat: () => false,
}));

// The screen subscribes to the post-download audio pass for a per-row
// "Optimizing audio…" line. Mocked as idle: loading the real module here
// would drag expo-file-system (and through it expo-modules-core, which only
// exists inside a real RN runtime) into a node test environment.
vi.mock("../lib/audioCompatPass", () => ({
  getAudioPassState: () => null,
  subscribeToAudioPass: () => () => {},
}));

// --- @evey/store / @evey/api (active profile used to gate each row) -------------------
// An unrestricted adult profile by default - none of the existing rows in
// these tests carry auth-context fields, so canPlayOffline allows them all
// (legacy entries are never retroactively blocked - see offlineAccess.ts).
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { profileId: string | null }) => unknown) => selector({ profileId: "profile-1" }),
}));
const useUserServiceListProfilesMock = vi.fn(() => ({ data: { profiles: [{ id: "profile-1", isKids: false }] } }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceGetCurrentUser: () => ({ data: { id: "user-1" } }),
  useUserServiceListProfiles: (...a: unknown[]) => useUserServiceListProfilesMock(...a),
}));

// --- network reachability -------------------------------------------------------------
// Drives the routing decision below. Online by default, like the device the
// rest of these tests assume.
const useOnlineStatusMock = vi.fn(() => true);
vi.mock("../hooks/useOnlineStatus", () => ({
  useOnlineStatus: () => useOnlineStatusMock(),
}));

// -------------------------------------------------------------------------------------
import DownloadsScreen from "../../app/(auth)/downloads";

// The SAMPLE dates are fixed in the past, so they sit outside the 30-day
// offline TTL and canPlayOffline refuses them - fine for the rendering tests,
// useless for the routing ones, which need a row a tap is allowed to open.
const FRESH = {
  mediaItemId: "fresh",
  title: "Fresh Movie",
  filePath: "/downloads/fresh.mp4",
  fileSize: 1_000_000,
  downloadedAt: new Date(Date.now() - 60_000).toISOString(),
};

const SAMPLE = [
  { mediaItemId: "m1", title: "Movie 1", filePath: "/downloads/m1.mp4", fileSize: 1_500_000, downloadedAt: "2026-01-01T00:00:00.000Z" },
  { mediaItemId: "m2", title: "Movie 2", filePath: "/downloads/m2.mp4", fileSize: 2_500_000, downloadedAt: "2026-01-02T00:00:00.000Z" },
];

beforeEach(() => {
  vi.clearAllMocks();
  getDownloadsMock.mockResolvedValue(SAMPLE);
  getTotalDownloadSizeMock.mockResolvedValue(4_000_000);
  deleteDownloadMock.mockResolvedValue(undefined);
  getInFlightDownloadsMock.mockReturnValue([]);
  useUserServiceListProfilesMock.mockReturnValue({ data: { profiles: [{ id: "profile-1", isKids: false }] } });
  useOnlineStatusMock.mockReturnValue(true);
});

describe("DownloadsScreen", () => {
  it("renders the downloaded items with title and size", async () => {
    render(<DownloadsScreen />);
    await waitFor(() => {
      expect(screen.getByText("Movie 1")).toBeTruthy();
      expect(screen.getByText("Movie 2")).toBeTruthy();
    });
  });

  // The reported parcours: download a title, go offline, tap it. Routing to
  // the media page asked the server for a screen it could no longer serve, so
  // the one file already on the device was unreachable from the app.
  it("opens the player directly when offline", async () => {
    useOnlineStatusMock.mockReturnValue(false);
    getDownloadsMock.mockResolvedValue([FRESH]);
    render(<DownloadsScreen />);
    await waitFor(() => expect(screen.getByText("Fresh Movie")).toBeTruthy());

    await act(async () => {
      fireEvent.click(screen.getByLabelText("Fresh Movie"));
    });

    expect(pushMock).toHaveBeenCalledWith("/player/fresh");
  });

  it("keeps opening the media page when online", async () => {
    getDownloadsMock.mockResolvedValue([FRESH]);
    render(<DownloadsScreen />);
    await waitFor(() => expect(screen.getByText("Fresh Movie")).toBeTruthy());

    await act(async () => {
      fireEvent.click(screen.getByLabelText("Fresh Movie"));
    });

    expect(pushMock).toHaveBeenCalledWith("/media/fresh");
  });

  it("still refuses to open a download this profile may not play, offline included", async () => {
    useOnlineStatusMock.mockReturnValue(false);
    getDownloadsMock.mockResolvedValue([
      { mediaItemId: "old", title: "Old Movie", filePath: "/downloads/old.mp4", fileSize: 1000, downloadedAt: "2020-01-01T00:00:00.000Z" },
    ]);
    render(<DownloadsScreen />);
    await waitFor(() => expect(screen.getByText("Old Movie")).toBeTruthy());

    await act(async () => {
      fireEvent.click(screen.getByLabelText("Old Movie"));
    });

    expect(pushMock).not.toHaveBeenCalled();
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

  it("shows an in-flight download as a placeholder row ahead of completed ones", async () => {
    getInFlightDownloadsMock.mockReturnValue([{ mediaItemId: "m3", title: "Movie 3 (downloading)" }]);

    render(<DownloadsScreen />);

    await waitFor(() => {
      expect(screen.getByText("Movie 3 (downloading)")).toBeTruthy();
      expect(screen.getByText("Movie 1")).toBeTruthy();
    });
    // No delete action while it's still downloading - nothing to delete yet.
    expect(screen.getAllByLabelText("Delete download")).toHaveLength(2);
  });

  it("does not duplicate a download that is both in-flight and already in the completed index", async () => {
    getInFlightDownloadsMock.mockReturnValue([{ mediaItemId: "m1", title: "Movie 1" }]);

    render(<DownloadsScreen />);

    await waitFor(() => expect(screen.getByText("Movie 1")).toBeTruthy());
    expect(screen.getAllByText("Movie 1")).toHaveLength(1);
  });

  it("marks an expired download with a badge but keeps it deletable", async () => {
    getDownloadsMock.mockResolvedValue([
      { mediaItemId: "old", title: "Old Movie", filePath: "/downloads/old.mp4", fileSize: 1000, downloadedAt: "2020-01-01T00:00:00.000Z" },
    ]);

    render(<DownloadsScreen />);

    await waitFor(() => expect(screen.getByText("Old Movie")).toBeTruthy());
    expect(screen.getByText("Expired - re-download to watch offline again")).toBeTruthy();
    expect(screen.getAllByLabelText("Delete download")).toHaveLength(1);
  });

  it("marks a download made by a different, more restricted profile with a badge", async () => {
    getDownloadsMock.mockResolvedValue([
      {
        mediaItemId: "adult-dl",
        title: "Adult Movie",
        filePath: "/downloads/adult-dl.mp4",
        fileSize: 1000,
        downloadedAt: new Date().toISOString(),
        downloadedByProfileId: "someone-else",
        downloadedByIsKids: false,
      },
    ]);
    useUserServiceListProfilesMock.mockReturnValue({
      data: { profiles: [{ id: "profile-1", isKids: true, maxAge: 10 }] },
    });

    render(<DownloadsScreen />);

    await waitFor(() => expect(screen.getByText("Adult Movie")).toBeTruthy());
    expect(screen.getByText("Downloaded by another profile")).toBeTruthy();
  });
});
