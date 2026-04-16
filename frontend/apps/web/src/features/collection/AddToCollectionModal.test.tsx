import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockListCollections = vi.fn(() => ({ data: { collections: [] }, isLoading: false }));

function selCollections(raw: { data?: { collections?: unknown[] }; [k: string]: unknown }, opts: { query?: { select?: (d: unknown) => unknown } }) {
  const s = opts?.query?.select;
  return { ...raw, data: raw?.data !== undefined && s ? s(raw.data) : raw?.data };
}

vi.mock("@evey/api", () => ({
  collectionServiceAddCollectionItem: vi.fn(),
  collectionServiceCreateCollection: vi.fn(),
  collectionServiceUpdateCollection: vi.fn(),
  collectionServiceDeleteCollection: vi.fn(),
  useCollectionServiceListCollections: (opts: { query?: { select?: (d: unknown) => unknown } }) => selCollections(mockListCollections(), opts),
  useCollectionServiceGetCollection: () => ({ data: undefined }),
  useCollectionServiceGetCollectionItems: () => ({ data: { items: [], total: 0 } }),
  getCollectionServiceListCollectionsQueryKey: () => ["collections"],
  getCollectionServiceGetCollectionQueryKey: (id: string) => ["collection", id],
  getCollectionServiceGetCollectionItemsQueryKey: (id: string) => ["collection-items", id],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: { open: boolean; children: React.ReactNode; title?: string }) =>
    open ? (
      <div role="dialog" aria-label={title}>
        {title && <h2>{title}</h2>}
        {children}
      </div>
    ) : null,
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, disabled }: { children: React.ReactNode; disabled?: boolean }) => (
    <button disabled={disabled}>{children}</button>
  ),
}));

vi.mock("@/components/ui/Spinner", () => ({
  Spinner: () => <div role="status" aria-label="Loading" />,
}));

function createWrapper(qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

describe("AddToCollectionModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders nothing when closed", async () => {
    
    mockListCollections.mockReturnValue({ data: { collections: [] }, isLoading: false });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={false} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper() }
    );

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows spinner while loading", async () => {
    
    // Never resolves during this test
    mockListCollections.mockReturnValue({ data: undefined, isLoading: true });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByRole("status", { name: "Loading" })).toBeInTheDocument();
  });

  it("shows empty message when no collections exist", async () => {
    
    mockListCollections.mockReturnValue({ data: { collections: [] }, isLoading: false });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["collections"], { collections: [] });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper(qc) }
    );

    expect(screen.getByText("No collections available.")).toBeInTheDocument();
  });

  it("renders collection buttons when collections exist", async () => {
    mockListCollections.mockReturnValue({
      data: {
        collections: [
          { id: "c1", name: "Favorites", createdAt: "", updatedAt: "" },
          { id: "c2", name: "Watch Later", createdAt: "", updatedAt: "" },
        ],
      },
      isLoading: false,
    });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("Favorites")).toBeInTheDocument();
    expect(screen.getByText("Watch Later")).toBeInTheDocument();
  });

  it("calls AddCollectionItem RPC and onClose on successful add", async () => {
    const { collectionServiceAddCollectionItem } = await import("@evey/api");
    const { toast } = await import("@evey/store");
    vi.mocked(collectionServiceAddCollectionItem).mockResolvedValue({});
    mockListCollections.mockReturnValue({ data: { collections: [{ id: "c1", name: "Favorites", createdAt: "", updatedAt: "" }] }, isLoading: false });

    const onClose = vi.fn();
    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={onClose} mediaItemIds={["m1", "m2"]} />,
      { wrapper: createWrapper() }
    );

    fireEvent.click(screen.getByText("Favorites"));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(vi.mocked(collectionServiceAddCollectionItem)).toHaveBeenCalledWith(
      "c1",
      { mediaItemId: "m1", sortOrder: 0 }
    );
    expect(vi.mocked(collectionServiceAddCollectionItem)).toHaveBeenCalledWith(
      "c1",
      { mediaItemId: "m2", sortOrder: 0 }
    );
    expect(vi.mocked(toast.success)).toHaveBeenCalledWith(expect.stringContaining("Favorites"));
  });

  it("shows error toast when API call fails", async () => {
    const { collectionServiceAddCollectionItem } = await import("@evey/api");
    const { toast } = await import("@evey/store");
    vi.mocked(collectionServiceAddCollectionItem).mockRejectedValue(new Error("Network error"));
    mockListCollections.mockReturnValue({ data: { collections: [{ id: "c1", name: "Favorites", createdAt: "", updatedAt: "" }] }, isLoading: false });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper() }
    );

    fireEvent.click(screen.getByText("Favorites"));

    await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith("Failed to add to collection"));
  });
});
