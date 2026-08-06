import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const getMediaItem = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", () => ({
  libraryServiceGetMediaItem: (id: string, signal?: AbortSignal) => getMediaItem(id, signal),
}));

// MediaDetail itself is out of scope for this fix - stub it so this test only
// exercises MediaPage's own loading/error/not-found branching.
vi.mock("@/features/library/MediaDetail", () => ({
  MediaDetail: ({ item }: { item: { title?: string } }) => React.createElement("div", { "data-testid": "media-detail" }, item.title ?? "item"),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getMediaPage(mediaId = "m1", edit = false) {
  const mod = await import("./$mediaId.lazy");
  (mod.Route as unknown as { useParams: () => { mediaId: string } }).useParams = () => ({ mediaId });
  (mod.Route as unknown as { useSearch: () => { edit?: boolean } }).useSearch = () => ({ edit });
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("MediaPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the media item once the fetch resolves", async () => {
    getMediaItem.mockResolvedValue({ id: "m1", title: "Some Movie" });
    const MediaPage = await getMediaPage();
    render(React.createElement(MediaPage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByTestId("media-detail")).toHaveTextContent("Some Movie"));
  });

  it("shows a 'not found' dead end with a way out (go home) on a 404 - no retry, since retrying can't help", async () => {
    getMediaItem.mockRejectedValue(Object.assign(new Error("nope"), { httpStatus: 404 }));
    const MediaPage = await getMediaPage();
    render(React.createElement(MediaPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Media item not found.")).toBeInTheDocument());
    expect(screen.queryByText("Retry")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Go Home" }));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/home" });
  });

  it("shows a distinct, retryable error state on a 500 - never the 'not found' message", async () => {
    const refetchable = Object.assign(new Error("boom"), { httpStatus: 500 });
    getMediaItem.mockRejectedValue(refetchable);
    const MediaPage = await getMediaPage();
    render(React.createElement(MediaPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Something went wrong")).toBeInTheDocument());
    expect(screen.queryByText("Media item not found.")).not.toBeInTheDocument();

    getMediaItem.mockResolvedValueOnce({ id: "m1", title: "Some Movie" });
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.getByTestId("media-detail")).toBeInTheDocument());
  });

  it("shows the offline copy when the failure carries no httpStatus (network down)", async () => {
    getMediaItem.mockRejectedValue(new TypeError("Failed to fetch"));
    const MediaPage = await getMediaPage();
    render(React.createElement(MediaPage), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("Offline")).toBeInTheDocument());
  });
});
