import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";

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

const api = vi.hoisted(() => ({
  myRequests: { data: { items: [] }, isLoading: false } as Record<string, unknown>,
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
  useMediaRequestServiceListMyRequests: () => api.myRequests,
  getMediaRequestServiceListMyRequestsQueryKey: () => ["/api/v1/requests/mine"],
}));

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { Route } from "./requests.lazy";

function renderPage() {
  const Page = (Route as unknown as { options: { component: () => React.ReactElement } }).options.component;
  return render(<Page />);
}

describe("RequestsPage", () => {
  it("does not claim the viewer has no requests when the list could not be read", () => {
    api.myRequests = { data: undefined, isLoading: false, isError: true, error: new Error("down"), refetch: vi.fn() };
    try {
      renderPage();
      // The "Mine" tab is the one that reads the list; an unread list must not
      // render as an empty one - it is also what hides the "already requested"
      // badge on the search tab, so the viewer re-submits what they already asked for.
      // i18n is not initialised in this harness, so the catalogue KEYS render.
      fireEvent.click(screen.getByRole("button", { name: "requests.tabMine" }));
      expect(screen.queryByText("requests.noRequests")).not.toBeInTheDocument();
    } finally {
      api.myRequests = { data: { items: [] }, isLoading: false };
    }
  });
});
