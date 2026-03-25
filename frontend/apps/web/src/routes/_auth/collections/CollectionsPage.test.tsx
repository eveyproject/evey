import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, to }: { children: React.ReactNode; to?: string; params?: Record<string, string> }) =>
    React.createElement("a", { href: to }, children),
}));

vi.mock("@/features/collection/api", () => ({
  useCollections: vi.fn(() => ({ data: undefined, isLoading: true })),
  useDeleteCollection: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}));

vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title),
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick }: { children: React.ReactNode; onClick?: () => void }) =>
    React.createElement("button", { onClick }, children),
}));

vi.mock("@/features/collection/SmartCollectionDialog", () => ({
  SmartCollectionDialog: () => null,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockCollections = [
  { id: "c1", name: "Marvel Cinematic Universe", posterUrl: null, createdAt: "", updatedAt: "" },
  { id: "c2", name: "Star Wars", posterUrl: null, createdAt: "", updatedAt: "" },
];

describe("CollectionsPage", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows skeleton cards while loading", async () => {
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    const { container } = render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(0);
  });

  it("shows empty state when no collections", async () => {
    const { useCollections } = await import("@/features/collection/api");
    vi.mocked(useCollections).mockReturnValue({ data: { collections: [] }, isLoading: false } as unknown as ReturnType<typeof useCollections>);
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("No collections yet")).toBeInTheDocument();
  });

  it("renders collection names when data is loaded", async () => {
    const { useCollections } = await import("@/features/collection/api");
    vi.mocked(useCollections).mockReturnValue({ data: { collections: mockCollections as never }, isLoading: false } as unknown as ReturnType<typeof useCollections>);
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    await waitFor(() => {
      expect(screen.getByText("Marvel Cinematic Universe")).toBeInTheDocument();
      expect(screen.getByText("Star Wars")).toBeInTheDocument();
    });
  });

  it("shows initial letter fallback when no posterUrl", async () => {
    const { useCollections } = await import("@/features/collection/api");
    vi.mocked(useCollections).mockReturnValue({ data: { collections: mockCollections as never }, isLoading: false } as unknown as ReturnType<typeof useCollections>);
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    await waitFor(() => {
      expect(screen.getByText("M")).toBeInTheDocument();
      expect(screen.getByText("S")).toBeInTheDocument();
    });
  });

  it("renders Collections heading", async () => {
    const { useCollections } = await import("@/features/collection/api");
    vi.mocked(useCollections).mockReturnValue({ data: { collections: [] }, isLoading: false } as unknown as ReturnType<typeof useCollections>);
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("heading", { name: /Collections/i })).toBeInTheDocument();
  });

  it("renders New smart collection button", async () => {
    const { useCollections } = await import("@/features/collection/api");
    vi.mocked(useCollections).mockReturnValue({ data: { collections: [] }, isLoading: false } as unknown as ReturnType<typeof useCollections>);
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /New smart collection/i })).toBeInTheDocument();
  });

  it("separates smart collections from TMDB collections", async () => {
    const { useCollections } = await import("@/features/collection/api");
    const mixed = [
      { id: "s1", name: "Action Movies", rules: [{ field: "genre", operator: "eq", value: "Action" }], createdAt: "", updatedAt: "" },
      { id: "c1", name: "Marvel", posterUrl: null, createdAt: "", updatedAt: "" },
    ];
    vi.mocked(useCollections).mockReturnValue({ data: { collections: mixed as never }, isLoading: false } as unknown as ReturnType<typeof useCollections>);
    const mod = await import("./index.lazy");
    const CollectionsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(CollectionsPage), { wrapper: createWrapper() });
    await waitFor(() => {
      expect(screen.getByText("Action Movies")).toBeInTheDocument();
      expect(screen.getByText("Marvel")).toBeInTheDocument();
      expect(screen.getByText("Smart Collections")).toBeInTheDocument();
      expect(screen.getByText("TMDB Collections")).toBeInTheDocument();
    });
  });
});
