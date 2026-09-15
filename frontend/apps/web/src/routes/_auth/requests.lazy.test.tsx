import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// createLazyFileRoute is the only export of the module; keep its component
// reachable so the page can be rendered without a router.
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (options: Record<string, unknown>) => ({ options }),
  Link: ({ children, to, ...rest }: { children: React.ReactNode; to: string; [k: string]: unknown }) => (
    <a href={to} {...rest}>{children}</a>
  ),
}));

vi.mock("@tanstack/react-query", async (importActual) => ({
  ...(await importActual<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

// The list is paged through the real useInfiniteQuery; the fetcher is what
// the tests script.
const api = vi.hoisted(() => ({
  listMyRequests: vi.fn<(...args: unknown[]) => Promise<{ items: unknown[] }>>(() => Promise.resolve({ items: [] })),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useMediaRequestServiceGetRequestPolicy: () => ({
    data: { canRequest: true, remaining: 5 },
    isLoading: false,
    isError: false,
  }),
  useMediaRequestServiceSearchTitles: () => ({ data: undefined, isFetching: false }),
  useMediaRequestServiceCreateRequest: () => ({ mutate: vi.fn(), isPending: false }),
  mediaRequestServiceListMyRequests: (...args: unknown[]) => api.listMyRequests(...args),
  getMediaRequestServiceListMyRequestsQueryKey: () => ["/api/v1/requests/mine"],
}));

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { Route } from "./requests.lazy";

function renderPage() {
  const Page = (Route as unknown as { options: { component: () => React.ReactElement } }).options.component;
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Page />
    </QueryClientProvider>,
  );
}

describe("RequestsPage", () => {
  it("does not claim the viewer has no requests when the list could not be read", async () => {
    api.listMyRequests.mockRejectedValueOnce(new Error("down"));
    renderPage();
    // The "Mine" tab is the one that reads the list; an unread list must not
    // render as an empty one - it is also what hides the "already requested"
    // badge on the search tab, so the viewer re-submits what they already asked for.
    // i18n is not initialised in this harness, so the catalogue KEYS render.
    fireEvent.click(screen.getByRole("button", { name: "My Requests" }));
    await waitFor(() => expect(api.listMyRequests).toHaveBeenCalled());
    await waitFor(() => expect(screen.queryByText("requests.noRequests")).not.toBeInTheDocument());
    // The failed read is reported, not swallowed.
    expect(await screen.findByRole("button", { name: /retry|réessayer|common\.retry/i })).toBeInTheDocument();
  });

  // The page used to ask for nothing and get the server's default 50, past
  // which older requests were gone. It now pages by 50 behind a sentinel.
  it("pages the list by 50 and fetches the next page from the sentinel", async () => {
    const page = (start: number, n: number) =>
      Array.from({ length: n }, (_, i) => ({ id: `r${start + i}`, title: `Title ${start + i}`, mediaType: "movie", status: "pending" }));
    api.listMyRequests.mockImplementation((params: unknown) => {
      const offset = (params as { offset: number }).offset;
      return Promise.resolve({ items: offset === 0 ? page(0, 50) : page(50, 2) });
    });
    let observed: ((entries: { isIntersecting: boolean }[]) => void) | undefined;
    class FakeObserver {
      constructor(cb: (entries: { isIntersecting: boolean }[]) => void) { observed = cb; }
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal("IntersectionObserver", FakeObserver);
    try {
      renderPage();
      fireEvent.click(screen.getByRole("button", { name: "My Requests" }));
      expect(await screen.findByText("Title 49")).toBeInTheDocument();
      expect(api.listMyRequests).toHaveBeenCalledWith({ limit: 50, offset: 0 }, expect.anything());
      expect(screen.queryByText("Title 50")).not.toBeInTheDocument();

      observed?.([{ isIntersecting: true }]);
      expect(await screen.findByText("Title 51")).toBeInTheDocument();
      expect(api.listMyRequests).toHaveBeenCalledWith({ limit: 50, offset: 50 }, expect.anything());
    } finally {
      vi.unstubAllGlobals();
      api.listMyRequests.mockImplementation(() => Promise.resolve({ items: [] }));
    }
  });
});
