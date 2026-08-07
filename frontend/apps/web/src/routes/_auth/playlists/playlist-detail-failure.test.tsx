import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// A playlist that fails to load is not a playlist that does not exist. The page
// used to render "playlist not found" for both, so a server that did not answer
// told the user their playlist was gone - and gave them no way to try again.

const playlistDetail = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => ({
    ...def,
    useParams: () => ({ playlistId: "pl-1" }),
  }),
  Link: ({ children, to }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: to }, children),
  // BackButton reaches for the router once the page actually renders - only
  // the success case gets that far.
  useRouter: () => ({ history: { back: vi.fn() }, navigate: vi.fn() }),
  useCanGoBack: () => false,
}));

vi.mock("@/features/playlist/api", () => ({
  usePlaylistDetail: (...args: unknown[]) => playlistDetail(...args),
  useRemoveFromPlaylist: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("@/features/playlist/SmartPlaylistRulesEditor", () => ({ SmartPlaylistRulesEditor: () => null }));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel: (s: { accessToken: string | null }) => unknown) => sel({ accessToken: "tok" }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function renderPage() {
  const mod = await import("./$playlistId.lazy");
  const PlaylistDetail = (mod.Route as unknown as { component: React.ComponentType }).component;
  render(React.createElement(PlaylistDetail), { wrapper: createWrapper() });
}

describe("PlaylistDetail - a failed load is not a missing playlist", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows a retryable failure, never 'not found', when the fetch errors", async () => {
    const refetch = vi.fn();
    playlistDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error("boom"),
      refetch,
    });

    await renderPage();

    await waitFor(() => expect(screen.getByTestId("fetch-error-state")).toBeInTheDocument());
    // The lie the fix removes.
    expect(screen.queryByText("media.playlist.notFound")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "common.retry" }));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("still says 'not found' when the server answered and the playlist is genuinely absent", async () => {
    playlistDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    await renderPage();

    await waitFor(() => expect(screen.getByText("media.playlist.notFound")).toBeInTheDocument());
    expect(screen.queryByTestId("fetch-error-state")).toBeNull();
  });

  it("renders the playlist when it loads", async () => {
    playlistDetail.mockReturnValue({
      data: { name: "Some Playlist", type: "manual", items: [], rules: [] },
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
    });

    await renderPage();

    await waitFor(() => expect(screen.getByText("Some Playlist")).toBeInTheDocument());
    expect(screen.queryByTestId("fetch-error-state")).toBeNull();
  });
});
