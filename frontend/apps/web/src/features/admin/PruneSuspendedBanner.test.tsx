import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PruneSuspendedBanner } from "./PruneSuspendedBanner";

const mockFetch = vi.fn();
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  toast,
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => mockFetch(...args),
}));

const libraries = [{ id: "lib-1", name: "Movies" }];

function renderBanner() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <PruneSuspendedBanner libraries={libraries} />
    </QueryClientProvider>,
  );
}

describe("PruneSuspendedBanner", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    toast.success.mockReset();
  });

  it("warns about a suspended root and confirms the deletion after a dialog", async () => {
    mockFetch.mockImplementation(({ url, method }: { url: string; method: string }) => {
      if (method === "POST") return Promise.resolve({ status: "started" });
      expect(url).toBe("/api/v1/admin/libraries/prune-suspended");
      return Promise.resolve({ suspensions: [{ libraryId: "lib-1", root: "/pool", missing: 30, total: 90, snapshot: "snap-1" }] });
    });
    renderBanner();
    const warning = await screen.findByTestId("prune-suspended-lib-1-/pool");
    expect(warning).toHaveTextContent("30 files missing under /pool, deletion suspended");

    fireEvent.click(screen.getByRole("button", { name: "Confirm deletion" }));
    // The dialog asks first: nothing is sent yet.
    expect(mockFetch).not.toHaveBeenCalledWith(expect.objectContaining({ method: "POST" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Confirm deletion" }));

    await waitFor(() =>
      expect(mockFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: "/api/v1/admin/libraries/lib-1/prune/confirm",
          method: "POST",
          // The confirmation names the root and the set on screen, nothing wider.
          data: { root: "/pool", snapshot: "snap-1" },
        }),
      ),
    );
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  it("renders nothing when no root is held", async () => {
    mockFetch.mockResolvedValue({ suspensions: [] });
    renderBanner();
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByTestId("prune-suspended")).toBeNull();
  });
});
