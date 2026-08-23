import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, to }: { children: React.ReactNode; to?: string; params?: Record<string, string> }) =>
    React.createElement("a", { href: to }, children),
}));

const mockListCollections = vi.fn(() => ({ data: undefined, isLoading: true }));

vi.mock("@evey/api", () => ({
  useCollectionServiceListCollections: (...args: unknown[]) => mockListCollections(...args),
}));

// The page reads the list through the shared module now, so the mock must
// carry that hook too - the same shape the screen actually imports.
vi.mock("@/features/collection/api", () => ({
  useCollections: () => mockListCollections(),
  useDeleteCollection: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}));

vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title),
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, ...rest }: { children: React.ReactNode; onClick?: () => void }) =>
    React.createElement("button", { onClick, ...rest }, children),
}));

let mockIsAdmin = true;
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { isAdmin: boolean }) => unknown) => sel({ isAdmin: mockIsAdmin }),
}));

vi.mock("@/features/collection/SmartCollectionDialog", () => ({
  SmartCollectionDialog: () => null,
}));

vi.mock("@/features/collection/CollectionDialog", () => ({
  CollectionDialog: () => null,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockCollections = [
  { id: "c1", name: "Marvel Cinematic Universe", posterUrl: null, createdAt: "", updatedAt: "" },
  { id: "c2", name: "Some Movie", posterUrl: null, createdAt: "", updatedAt: "" },
];

async function renderPage() {
  const mod = await import("./index.lazy");
  const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
  return render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
}

describe("CollectionsPage", () => {
  beforeEach(() => { vi.clearAllMocks(); mockIsAdmin = true; mockListCollections.mockReturnValue({ data: undefined, isLoading: true }); });

  // Every write here is RoleAdmin server-side, so a viewer offered the button
  // only got a 403 once committed to the gesture. The collections themselves
  // stay readable.
  it("gives a viewer no write control, while still listing the collections", async () => {
    mockIsAdmin = false;
    mockListCollections.mockReturnValue({
      data: { collections: [...mockCollections, { id: "s1", name: "Recent", rules: [{ field: "year", operator: "gte", value: "2020" }], createdAt: "", updatedAt: "" }] },
      isLoading: false,
    });
    await renderPage();
    await waitFor(() => expect(screen.getByText("Some Movie")).toBeInTheDocument());
    expect(screen.queryByTestId("new-smart-collection")).not.toBeInTheDocument();
    expect(screen.queryByTestId("new-collection")).not.toBeInTheDocument();
    expect(screen.queryByTestId("collection-delete-c1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("collection-edit-c1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("smart-edit-s1")).not.toBeInTheDocument();
    expect(screen.queryByTestId("smart-delete-s1")).not.toBeInTheDocument();
  });

  it("gives an admin all six write controls", async () => {
    mockIsAdmin = true;
    mockListCollections.mockReturnValue({
      data: { collections: [...mockCollections, { id: "s1", name: "Recent", rules: [{ field: "year", operator: "gte", value: "2020" }], createdAt: "", updatedAt: "" }] },
      isLoading: false,
    });
    await renderPage();
    await waitFor(() => expect(screen.getByTestId("new-smart-collection")).toBeInTheDocument());
    expect(screen.getByTestId("new-collection")).toBeInTheDocument();
    expect(screen.getByTestId("collection-delete-c1")).toBeInTheDocument();
    expect(screen.getByTestId("collection-edit-c1")).toBeInTheDocument();
    expect(screen.getByTestId("smart-edit-s1")).toBeInTheDocument();
    expect(screen.getByTestId("smart-delete-s1")).toBeInTheDocument();
  });

  it("shows skeleton cards while loading", async () => {
    const { container } = await renderPage();
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows empty state when no collections", async () => {
    mockListCollections.mockReturnValue({ data: { collections: [] }, isLoading: false });
    await renderPage();
    expect(screen.getByText("No collections yet")).toBeInTheDocument();
  });

  it("renders collection names when data is loaded", async () => {
    mockListCollections.mockReturnValue({ data: { collections: mockCollections }, isLoading: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText("Marvel Cinematic Universe")).toBeInTheDocument();
      expect(screen.getByText("Some Movie")).toBeInTheDocument();
    });
  });

  it("shows initial letter fallback when no posterUrl", async () => {
    mockListCollections.mockReturnValue({ data: { collections: mockCollections }, isLoading: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText("M")).toBeInTheDocument();
      expect(screen.getByText("S")).toBeInTheDocument();
    });
  });

  it("renders Collections heading", async () => {
    mockListCollections.mockReturnValue({ data: { collections: [] }, isLoading: false });
    await renderPage();
    expect(screen.getByRole("heading", { name: /Collections/i })).toBeInTheDocument();
  });

  it("renders New smart collection button", async () => {
    mockListCollections.mockReturnValue({ data: { collections: [] }, isLoading: false });
    await renderPage();
    expect(screen.getByRole("button", { name: /New smart collection/i })).toBeInTheDocument();
  });

  it("separates smart collections from TMDB collections", async () => {
    const mixed = [
      { id: "s1", name: "Action Movies", rules: [{ field: "genre", operator: "eq", value: "Action" }], createdAt: "", updatedAt: "" },
      { id: "c1", name: "Marvel", posterUrl: null, createdAt: "", updatedAt: "" },
    ];
    mockListCollections.mockReturnValue({ data: { collections: mixed }, isLoading: false });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText("Action Movies")).toBeInTheDocument();
      expect(screen.getByText("Marvel")).toBeInTheDocument();
      expect(screen.getByText("Smart Collections")).toBeInTheDocument();
      expect(screen.getByText("TMDB Collections")).toBeInTheDocument();
    });
  });

  it("shows the visible title count on a poster card", async () => {
    // count: 1 sidesteps this fixture's stale i18next v3 "_plural" suffix
    // (the installed i18next resolves "_one"/"_other" instead, so any count
    // other than 1 falls back to the unsuffixed singular string here).
    mockListCollections.mockReturnValue({
      data: { collections: [{ id: "c1", name: "Marvel", posterUrl: null, itemCount: 1, createdAt: "", updatedAt: "" }] },
      isLoading: false,
    });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText("1 title")).toBeInTheDocument();
    });
  });

  it("does not show a title count when itemCount is absent or zero", async () => {
    mockListCollections.mockReturnValue({
      data: {
        collections: [
          { id: "c1", name: "Marvel", posterUrl: null, createdAt: "", updatedAt: "" },
          { id: "c2", name: "DC", posterUrl: null, itemCount: 0, createdAt: "", updatedAt: "" },
        ],
      },
      isLoading: false,
    });
    await renderPage();
    await waitFor(() => expect(screen.getByText("Marvel")).toBeInTheDocument());
    expect(screen.queryByText(/titles?$/)).not.toBeInTheDocument();
  });

  it("badges saga collections but not manual ones", async () => {
    mockListCollections.mockReturnValue({
      data: {
        collections: [
          { id: "c1", name: "Marvel", posterUrl: null, kind: "saga", itemCount: 12, createdAt: "", updatedAt: "" },
          { id: "c2", name: "My Favorites", posterUrl: null, kind: "manual", createdAt: "", updatedAt: "" },
        ],
      },
      isLoading: false,
    });
    await renderPage();
    await waitFor(() => {
      expect(screen.getByText("Saga")).toBeInTheDocument();
      expect(screen.getByText("Marvel")).toBeInTheDocument();
      expect(screen.getByText("My Favorites")).toBeInTheDocument();
    });
    expect(screen.getAllByText("Saga")).toHaveLength(1);
  });
});

describe("collectionSubtitle", () => {
  const t = (key: string, options?: Record<string, unknown>) =>
    options?.count !== undefined ? `${options.count} titles` : key;

  it("returns no parts for a manual collection with no known count", async () => {
    const { collectionSubtitle } = await import("./index.lazy");
    expect(collectionSubtitle(undefined, undefined, t)).toEqual({});
  });

  it("returns a badge for a saga collection", async () => {
    const { collectionSubtitle } = await import("./index.lazy");
    expect(collectionSubtitle("saga", undefined, t)).toEqual({ badge: "media.collections.saga" });
  });

  it("returns a count when itemCount is positive, regardless of kind", async () => {
    const { collectionSubtitle } = await import("./index.lazy");
    expect(collectionSubtitle("manual", 7, t)).toEqual({ count: "7 titles" });
  });

  it("omits the count when itemCount is zero", async () => {
    const { collectionSubtitle } = await import("./index.lazy");
    expect(collectionSubtitle("saga", 0, t)).toEqual({ badge: "media.collections.saga" });
  });

  it("combines badge and count for a saga with visible titles", async () => {
    const { collectionSubtitle } = await import("./index.lazy");
    expect(collectionSubtitle("saga", 3, t)).toEqual({ badge: "media.collections.saga", count: "3 titles" });
  });
});
