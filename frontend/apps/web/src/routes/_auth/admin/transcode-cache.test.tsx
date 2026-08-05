import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockUseQuery = vi.fn();
const mockServerCfg = { data: undefined, isLoading: false };
const mockInvalidateQueries = vi.fn();
const mockEveyFetch = vi.fn();
const mockToast = { success: vi.fn(), error: vi.fn() };

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
  useAdminSystemServiceGetSystemInfo: () => ({ data: undefined }),
  useTranscodeCacheServiceGetTranscodeCache: () => mockUseQuery(),
  transcodeCacheServiceClearTranscodeCache: vi.fn(),
  transcodeCacheServiceDeleteTranscodeCacheFile: vi.fn(),
  getTranscodeCacheServiceGetTranscodeCacheQueryKey: () => ["transcode-cache"],
  useServerConfigServiceListServerConfigEntries: () => mockServerCfg,
  serverConfigServiceSetServerConfigEntry: vi.fn(),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["server-config"],
}));

vi.mock("@evey/store", () => ({
  toast: mockToast,
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
  // The page renders a <Link> back to the admin dashboard; render it as a
  // plain anchor (the router is fully mocked in this suite).
  Link: ({ children, ...props }: { children?: React.ReactNode } & Record<string, unknown>) =>
    React.createElement("a", props as Record<string, unknown>, children),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getTranscodeCachePage() {
  const mod = await import("./transcode-cache.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const cacheData = {
  totalSize: 2147483648, // 2 GB
  fileCount: 3,
  files: [
    { name: "video1.ts", size: 1073741824, lastModified: "2025-01-01T00:00:00Z" },
    { name: "video2.ts", size: 536870912, lastModified: "2025-01-02T00:00:00Z" },
    { name: "video3.ts", size: 536870912, lastModified: "2025-01-03T00:00:00Z" },
  ],
};

describe("TranscodeCachePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseQuery.mockReturnValue({ data: null, isLoading: false });
  });

  it("renders page heading", async () => {
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    expect(screen.getByText("Transcode Cache")).toBeInTheDocument();
  });

  it("shows empty state when no files", async () => {
    mockUseQuery.mockReturnValue({ data: { totalSize: 0, fileCount: 0, files: [] }, isLoading: false });
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    expect(screen.getByText(/Cache is empty/i)).toBeInTheDocument();
  });

  it("renders file list and total size", async () => {
    mockUseQuery.mockReturnValue({ data: cacheData, isLoading: false });
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    expect(screen.getByText("video1.ts")).toBeInTheDocument();
    expect(screen.getByText("2.0 GB")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("shows Clear All button when files exist", async () => {
    mockUseQuery.mockReturnValue({ data: cacheData, isLoading: false });
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Clear All/i })).toBeInTheDocument();
  });

  it("clicking Clear All opens confirmation modal", async () => {
    mockUseQuery.mockReturnValue({ data: cacheData, isLoading: false });
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Clear All/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText(/Clear Transcode Cache/i)).toBeInTheDocument();
  });

  it("cancel in confirm modal closes it", async () => {
    mockUseQuery.mockReturnValue({ data: cacheData, isLoading: false });
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Clear All/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("each file row has a Delete button", async () => {
    mockUseQuery.mockReturnValue({ data: cacheData, isLoading: false });
    const TranscodeCachePage = await getTranscodeCachePage();
    render(React.createElement(TranscodeCachePage), { wrapper: createWrapper() });
    const deleteButtons = screen.getAllByRole("button", { name: /Delete/i });
    expect(deleteButtons).toHaveLength(cacheData.files.length);
  });
});
