import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ collections: [] });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={false} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper() }
    );

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows spinner while loading", async () => {
    const { eveyFetch } = await import("@evey/api");
    // Never resolves during this test
    vi.mocked(eveyFetch).mockReturnValue(new Promise(() => {}));

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper() }
    );

    expect(screen.getByRole("status", { name: "Loading" })).toBeInTheDocument();
  });

  it("shows empty message when no collections exist", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ collections: [] });

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
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ collections: [] });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["collections"], {
      collections: [
        { id: "c1", name: "Favorites", createdAt: "", updatedAt: "" },
        { id: "c2", name: "Watch Later", createdAt: "", updatedAt: "" },
      ],
    });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper(qc) }
    );

    expect(screen.getByText("Favorites")).toBeInTheDocument();
    expect(screen.getByText("Watch Later")).toBeInTheDocument();
  });

  it("calls eveyFetch POST and onClose on successful add", async () => {
    const { eveyFetch } = await import("@evey/api");
    const { toast } = await import("@evey/store");
    vi.mocked(eveyFetch).mockResolvedValue({});

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["collections"], {
      collections: [{ id: "c1", name: "Favorites", createdAt: "", updatedAt: "" }],
    });

    const onClose = vi.fn();
    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={onClose} mediaItemIds={["m1", "m2"]} />,
      { wrapper: createWrapper(qc) }
    );

    fireEvent.click(screen.getByText("Favorites"));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/collections/c1/items", method: "POST", data: { mediaItemId: "m1" } })
    );
    expect(vi.mocked(toast.success)).toHaveBeenCalledWith('2 items added to "Favorites"');
  });

  it("shows error toast when API call fails", async () => {
    const { eveyFetch } = await import("@evey/api");
    const { toast } = await import("@evey/store");
    vi.mocked(eveyFetch).mockRejectedValue(new Error("Network error"));

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    qc.setQueryData(["collections"], {
      collections: [{ id: "c1", name: "Favorites", createdAt: "", updatedAt: "" }],
    });

    const { AddToCollectionModal } = await import("./AddToCollectionModal");
    render(
      <AddToCollectionModal open={true} onClose={vi.fn()} mediaItemIds={["m1"]} />,
      { wrapper: createWrapper(qc) }
    );

    fireEvent.click(screen.getByText("Favorites"));

    await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalledWith("Failed to add to collection"));
  });
});
