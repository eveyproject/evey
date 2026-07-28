import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const librariesHook = vi.fn(() => ({ data: { libraries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() }));
const recentHook = vi.fn(() => ({ data: { items: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() }));
const feedConfigHook = vi.fn(() => ({ data: undefined, isError: false }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, ...props }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: props.to }, children),
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", () => ({
  useFeedServiceRecommended: () => ({ data: { items: [] }, isLoading: false }),
  useFeedServiceGetFeedConfig: (...args: unknown[]) => feedConfigHook(...(args as [])),
  useFeedServiceRecentlyAdded: () => recentHook(),
  useFeedServiceResumePlayback: () => ({ data: { items: [] }, isLoading: false }),
  useLibraryServiceListLibraries: () => librariesHook(),
  useLibraryServiceListMediaItems: () => ({ data: { items: [] }, isLoading: false }),
  useMusicServiceGetAlbums: () => ({ data: { albums: [] }, isLoading: false }),
  usePhotoServiceListPhotoAlbums: () => ({ data: { albums: [] }, isLoading: false }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: "p1" }),
}));

// CustomizePanel mounts unconditionally at the bottom of HomePage (even when
// closed) and pulls in its own set of feed-config hooks - stub it out so this
// test only exercises HomePage's own error/empty/content branching.
vi.mock("@/features/home/CustomizePanel", () => ({
  CustomizePanel: () => null,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getHomePage() {
  const mod = await import("./home.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("HomePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    librariesHook.mockReturnValue({ data: { libraries: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() });
    recentHook.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, error: null, refetch: vi.fn() });
    feedConfigHook.mockReturnValue({ data: undefined, isError: false });
  });

  it("shows the empty-library state with an action when there is no error and no content", async () => {
    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText(/your library is empty/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /go to libraries/i })).toBeInTheDocument();
  });

  it("shows a distinct error state with a retry action when the library fetch fails - not the empty-library message", async () => {
    const refetchLibraries = vi.fn();
    librariesHook.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: Object.assign(new Error("boom"), { httpStatus: 500 }), refetch: refetchLibraries });

    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText(/your library is empty/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetchLibraries).toHaveBeenCalled();
  });

  it("shows the offline copy when the failure carries no httpStatus (network down)", async () => {
    recentHook.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: new TypeError("Failed to fetch"), refetch: vi.fn() });

    const HomePage = await getHomePage();
    render(React.createElement(HomePage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Offline")).toBeInTheDocument());
  });
});
