import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const navigateMock = vi.fn();
const markUnwatched = vi.fn();
const markRead = vi.fn();
const markAllRead = vi.fn();
const notifications = vi.fn();
const infiniteQuery = vi.fn();

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a) }));
const authState = { profileId: "prf-1", accessToken: "t" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  useQueries: () => [],
  useQuery: () => ({ data: undefined }),
  useInfiniteQuery: () => infiniteQuery(),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] }, isLoading: false }),
  useLibraryServiceSearchMedia: () => ({ data: { items: [] }, isLoading: false }),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] }, isLoading: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutateAsync: markUnwatched }),
  useNotificationServiceListNotifications: () => notifications(),
  useNotificationServiceMarkRead: () => ({ mutate: markRead }),
  useNotificationServiceMarkAllRead: () => ({ mutate: markAllRead }),
  playbackServiceGetWatchHistory: vi.fn(),
  getPlaybackServiceGetWatchHistoryQueryKey: () => ["history"],
  getNotificationServiceListNotificationsQueryKey: () => ["notifications"],
  getNotificationServiceGetUnreadCountQueryKey: () => ["unread"],
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useCollections: () => ({ data: { collections: [] }, isLoading: false }),
  useCollection: () => ({ data: undefined, isLoading: false }),
  useFavoritesList: () => ({ data: [], isLoading: false }),
  usePlaylistList: () => ({ data: [], isLoading: false }),
  usePlaylistDetail: () => ({ data: undefined, isLoading: false }),
}));

import { HistoryScreen, NotificationsScreen } from "./Browse";

beforeEach(() => {
  navigateMock.mockReset();
  markUnwatched.mockReset().mockResolvedValue({});
  markRead.mockReset();
  markAllRead.mockReset();
  infiniteQuery.mockReturnValue({
    data: { pages: [{ items: [{ mediaItemId: "m-1", title: "Watched film", percentComplete: 42, positionMs: "60000", durationMs: "600000", lastPlayedAt: "2026-09-20T10:00:00Z", mediaType: "movie" }] }] },
    isLoading: false,
    fetchNextPage: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
  });
  notifications.mockReturnValue({
    data: { notifications: [{ id: "n-1", type: "media_added", mediaItemId: "m-9", createdAt: "2026-09-21T09:00:00Z" }] },
    isLoading: false,
  });
});

describe("the watch history", () => {
  it("names the title and offers to unwatch it", async () => {
    render(<HistoryScreen />);
    expect(screen.getByText("Watched film")).toBeTruthy();
    fireEvent.click(screen.getByText("media.library.markUnwatched"));
    expect(markUnwatched).toHaveBeenCalledWith({ mediaItemId: "m-1" });
  });

  it("opens the title it names", () => {
    render(<HistoryScreen />);
    fireEvent.click(screen.getByText("Watched film"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "media", mediaId: "m-1" });
  });
});

describe("the notifications", () => {
  it("marks one read and opens what it is about", () => {
    render(<NotificationsScreen />);
    // The row, not the heading or the mark-all button.
    fireEvent.click(document.querySelectorAll(".track-list .btn")[0]);
    expect(markRead).toHaveBeenCalledWith({ id: "n-1" });
    expect(navigateMock).toHaveBeenCalledWith({ name: "media", mediaId: "m-9" });
  });

  it("offers mark-all-read only while something is unread", () => {
    render(<NotificationsScreen />);
    expect(screen.getByText("common.notifications.markAllRead")).toBeTruthy();
    notifications.mockReturnValue({ data: { notifications: [{ id: "n-1", readAt: "2026-09-21T10:00:00Z" }] }, isLoading: false });
    render(<NotificationsScreen />);
    expect(screen.getAllByText("common.notifications.markAllRead")).toHaveLength(1);
  });
});
