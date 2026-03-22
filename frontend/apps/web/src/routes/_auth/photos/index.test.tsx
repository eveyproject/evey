import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

vi.mock("@evey/api", () => ({ eveyFetch: vi.fn() }));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: vi.fn(() => false),
}));
vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});
vi.mock("lucide-react", () => ({
  Images: () => React.createElement("span", null, "icon"),
  ChevronLeft: () => React.createElement("span"),
  ChevronRight: () => React.createElement("span"),
  X: () => React.createElement("span"),
  Trash2: () => React.createElement("span"),
}));
vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function setupQuery(overrides: Record<string, unknown> = {}) {
  const rq = await import("@tanstack/react-query");
  vi.spyOn(rq, "useQuery").mockImplementation(({ queryKey }: { queryKey: readonly unknown[] }) => {
    const [k0, k1] = queryKey as string[];
    if (k0 === "libraries" && k1 === "photos") return { data: overrides.libraries ?? [], isLoading: false } as never;
    if (k0 === "photos" && k1 === "albums") return { data: overrides.albums ?? [], isLoading: false } as never;
    if (k0 === "photos" && k1 === "list") return { data: overrides.photos ?? { photos: [], total: 0 }, isLoading: false } as never;
    return { data: undefined, isLoading: false } as never;
  });
}

describe("PhotosPage", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows loading skeleton when libraries loading", async () => {
    const rq = await import("@tanstack/react-query");
    vi.spyOn(rq, "useQuery").mockReturnValue({ data: undefined, isLoading: true } as never);
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });
    expect(document.querySelector(".animate-pulse")).toBeTruthy();
  });

  it("shows empty state when no libraries", async () => {
    await setupQuery({ libraries: [] });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText(/no photo libraries/i)).toBeInTheDocument());
  });

  it("shows albums view by default when library selected", async () => {
    await setupQuery({
      libraries: [{ id: "lib1", name: "My Photos" }],
      albums: [{ Name: "Vacation", LibraryID: "lib1", PhotoCount: 5, CoverPhotoID: "" }],
    });
    const mod = await import("./index.lazy");
    const PhotosPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PhotosPage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText("Vacation")).toBeInTheDocument());
  });
});
