import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

// Curating a collection was write-only until RemoveCollectionItem existed:
// adding shipped without a counterpart, so a wrong member was permanent.
// These cover the control that finally removes one, and who may see it.

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) =>
    Object.assign(def, {
      useParams: () => ({ collectionId: "col-1" }),
      useNavigate: () => vi.fn(),
    }),
  Link: ({ children, to }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: to }, children),
}));

const removeMutate = vi.fn();
const reorderMutate = vi.fn();
const isAdmin = { value: true };

vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { accessToken: string; isAdmin: boolean }) => unknown) =>
    sel({ accessToken: "tok", isAdmin: isAdmin.value }),
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/features/collection/api", () => ({
  // Shape copied from the real useCollection: { data: { collection, items, total } }.
  useCollection: () => ({
    data: {
      collection: { id: "col-1", name: "Matrix - Saga" },
      items: [
        { id: "mi-1", title: "Matrix", type: "movie" },
        { id: "mi-2", title: "Matrix Reloaded", type: "movie" },
      ],
      total: 2,
    },
    isLoading: false,
    isError: false,
  }),
  useDeleteCollection: () => ({ mutate: vi.fn(), isPending: false }),
  useRemoveCollectionItem: () => ({ mutate: removeMutate, isPending: false }),
  useReorderCollectionItems: () => ({ mutate: reorderMutate, isPending: false }),
}));

vi.mock("@/features/library/MediaGrid", () => ({
  MediaGrid: ({ children }: { children: React.ReactNode }) => React.createElement("div", null, children),
}));
vi.mock("@/features/library/MediaCard", () => ({
  MediaCard: ({ item }: { item: { title: string } }) => React.createElement("div", null, item.title),
}));
vi.mock("@/components/ui/EmptyState", () => ({ EmptyState: () => null }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading: _loading, variant: _variant, ...rest }: { children: React.ReactNode; onClick?: () => void; loading?: boolean; variant?: string }) =>
    React.createElement("button", { onClick, ...rest }, children),
}));
vi.mock("@/components/ui/BackButton", () => ({ BackButton: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

beforeEach(() => {
  removeMutate.mockClear();
  reorderMutate.mockClear();
  isAdmin.value = true;
  vi.stubGlobal("confirm", vi.fn(() => true));
});

describe("removing a title from a collection", () => {
  it("asks for confirmation and removes the exact title", async () => {
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    fireEvent.click(screen.getByTestId("collection-remove-mi-2"));
    expect(removeMutate).toHaveBeenCalledTimes(1);
    expect(removeMutate.mock.calls[0][0]).toBe("mi-2");
  });

  it("does nothing when the confirmation is declined", async () => {
    vi.stubGlobal("confirm", vi.fn(() => false));
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    fireEvent.click(screen.getByTestId("collection-remove-mi-1"));
    expect(removeMutate).not.toHaveBeenCalled();
  });

  it("hides the control from a non-admin - the RPC is admin-only", async () => {
    isAdmin.value = false;
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    expect(screen.queryByTestId("collection-remove-mi-1")).toBeNull();
  });
});

describe("deleting the collection itself", () => {
  it("offers the delete button to an admin", async () => {
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    expect(screen.getByTestId("collection-delete-btn")).toBeInTheDocument();
  });

  it("hides it from a non-admin, who can still read the collection", async () => {
    isAdmin.value = false;
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    expect(screen.queryByTestId("collection-delete-btn")).toBeNull();
    expect(screen.getByText("Matrix - Saga")).toBeInTheDocument();
  });
});

describe("reordering a collection", () => {
  it("sends the full new order with the moved title one place earlier", async () => {
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    fireEvent.click(screen.getByTestId("collection-up-mi-2"));
    expect(reorderMutate).toHaveBeenCalledTimes(1);
    expect(reorderMutate.mock.calls[0][0]).toEqual({
      mediaItemIds: ["mi-2", "mi-1"],
      name: "Matrix - Saga",
    });
  });

  it("cannot move the first title up nor the last one down", async () => {
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    expect(screen.getByTestId("collection-up-mi-1")).toBeDisabled();
    expect(screen.getByTestId("collection-down-mi-2")).toBeDisabled();
  });

  it("offers no reordering to a non-admin", async () => {
    isAdmin.value = false;
    const { Route } = await import("./$collectionId.lazy");
    render(React.createElement((Route as unknown as { component: React.ComponentType }).component));

    expect(screen.queryByTestId("collection-up-mi-2")).toBeNull();
  });
});
