import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

// DeleteCollection is RoleAdmin server-side, and this page offered its Delete
// button to everyone - a viewer got as far as the confirmation before the call
// came back 403.

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) =>
    Object.assign(def, {
      useParams: () => ({ filterId: "smart-1" }),
      useNavigate: () => vi.fn(),
    }),
  Link: ({ children, to }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: to }, children),
}));

const isAdmin = { value: true };

vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { isAdmin: boolean }) => unknown) => sel({ isAdmin: isAdmin.value }),
  toast: { success: vi.fn(), error: vi.fn() },
}));

// Shape copied from the real useCollection: { data, isLoading, isError, error,
// refetch } - the screen reads every one of them.
const failing = { value: false };
// Records how the screen asks for its page. A smart collection is rule-based,
// so it routinely matches more titles than one page holds - and this page used
// to take the default limit and never paginate, while displaying the server's
// real total in its header.
const collectionArgs: unknown[][] = [];
vi.mock("@/features/collection/api", () => ({
  useCollection: (...args: unknown[]) => {
    collectionArgs.push(args);
    return {
    data: failing.value
      ? undefined
      : {
          collection: { id: "smart-1", name: "Recent sci-fi", rules: [{ field: "year", operator: "gte", value: "2020" }] },
          items: [{ id: "mi-1", title: "Some Movie", type: "movie" }],
          total: 1,
        },
    isLoading: false,
    isError: failing.value,
    error: failing.value ? new Error("boom") : undefined,
    refetch: vi.fn(),
    };
  },
  useDeleteCollection: () => ({ mutate: vi.fn(), isPending: false }),
}));

vi.mock("@/features/library/MediaGrid", () => ({
  MediaGrid: ({ children }: { children: React.ReactNode }) => React.createElement("div", null, children),
}));
vi.mock("@/features/library/MediaCard", () => ({
  MediaCard: ({ item }: { item: { title: string } }) => React.createElement("div", null, item.title),
}));
vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", { "data-testid": "empty-state" }, title),
}));
vi.mock("@/components/ui/FetchErrorState", () => ({
  FetchErrorState: () => React.createElement("div", { "data-testid": "fetch-error-state" }),
}));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => null }));
vi.mock("@/components/ui/BackButton", () => ({ BackButton: () => null }));
vi.mock("@/components/ui/Badge", () => ({
  Badge: ({ children }: { children: React.ReactNode }) => React.createElement("span", null, children),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading: _loading, variant: _variant, ...rest }: { children: React.ReactNode; onClick?: () => void; loading?: boolean; variant?: string }) =>
    React.createElement("button", { onClick, ...rest }, children),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

async function renderPage() {
  const { Route } = await import("./smart/$filterId.lazy");
  return render(React.createElement((Route as unknown as { component: React.ComponentType }).component));
}

beforeEach(() => {
  isAdmin.value = true;
  failing.value = false;
});

describe("smart collection page", () => {
  it("offers the delete button to an admin", async () => {
    await renderPage();
    expect(screen.getByTestId("smart-delete-btn")).toBeInTheDocument();
  });

  it("hides it from a viewer, who still gets the results", async () => {
    isAdmin.value = false;
    await renderPage();
    expect(screen.queryByTestId("smart-delete-btn")).toBeNull();
    expect(screen.getByText("Recent sci-fi")).toBeInTheDocument();
    expect(screen.getByText("Some Movie")).toBeInTheDocument();
  });
});

// A server that did not answer is not a collection someone deleted. This
// screen claimed "collection not found" on any failure.
describe("smart collection page, when the load fails", () => {
  it("says the load failed instead of claiming the collection is gone", async () => {
    failing.value = true;
    await renderPage();
    expect(screen.getByTestId("fetch-error-state")).toBeInTheDocument();
    expect(screen.queryByTestId("empty-state")).toBeNull();
  });
  it("asks for an explicit page rather than taking whatever the default is", async () => {
    collectionArgs.length = 0;
    const mod = await import("./smart/$filterId.lazy");
    const Page = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(Page));

    expect(collectionArgs.length).toBeGreaterThan(0);
    const [, limit, offset] = collectionArgs[0] as [string, number, number];
    expect(limit, "a page size must be requested, not inherited from the hook default").toBeGreaterThan(0);
    expect(offset, "the first page starts at 0").toBe(0);
  });
});
