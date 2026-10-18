import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockList = vi.fn();
const mockRevoke = vi.fn();
const mockInvalidate = vi.fn();
let revokeOptions: { mutation?: { onSuccess?: () => void } } = {};

vi.mock("@evey/api", () => ({
  useShareServiceListShareLinks: () => mockList(),
  useShareServiceRevokeShareLink: (opts: typeof revokeOptions) => {
    revokeOptions = opts;
    return { mutate: mockRevoke, isPending: false };
  },
  getShareServiceListShareLinksQueryKey: () => ["/api/v1/admin/share-links"],
  useLibraryServiceGetMediaItem: (id: string) => ({ data: id === "m1" ? { title: "Caminandes" } : undefined }),
  apiErrorMessage: (_e: unknown, _t: unknown, fallback: string) => fallback,
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@tanstack/react-query", async (importActual) => ({
  ...(await importActual<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
}));
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
  Link: ({ children }: { children: React.ReactNode }) => React.createElement("a", null, children),
}));
vi.mock("@/hooks/useDemoMode", () => ({ useDemoLock: () => ({ demo: false, lock: {} }) }));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: React.ReactNode }) => (open ? React.createElement("div", { role: "dialog" }, children) : null),
}));

async function page() {
  const mod = await import("./share-links.lazy");
  const Page = (mod.Route as unknown as { component: React.ComponentType }).component;
  return render(<Page />);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockList.mockReturnValue({
    data: { items: [
      { id: "s1", mediaItemId: "m1", expiresAt: "2099-01-01T00:00:00Z", viewCount: 2, maxViews: 5 },
      { id: "s2", mediaItemId: "m2", expiresAt: "2000-01-01T00:00:00Z", viewCount: 0 },
    ] },
    isLoading: false, isError: false,
  });
});

describe("admin share links (web)", () => {
  it("lists every link with its title, views and an expired mark", async () => {
    await page();
    expect(screen.getByText("Caminandes")).toBeTruthy();
    expect(screen.getByText("2 / 5")).toBeTruthy();
    expect(screen.getByText("0 / unlimited")).toBeTruthy();
    expect(screen.getAllByText("expired")).toHaveLength(1);
  });

  it("revokes only after the confirmation, then refreshes the list", async () => {
    await page();
    fireEvent.click(screen.getAllByText("Revoke")[0]);
    expect(mockRevoke).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    fireEvent.click(dialog.querySelectorAll("button")[1]);
    expect(mockRevoke).toHaveBeenCalledWith({ id: "s1" });
    revokeOptions.mutation?.onSuccess?.();
    expect(mockInvalidate).toHaveBeenCalledWith({ queryKey: ["/api/v1/admin/share-links"] });
    expect(toast.success).toHaveBeenCalledWith("Share link revoked");
  });

  it("says when there is none", async () => {
    mockList.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false });
    await page();
    expect(screen.getByText("No share link.")).toBeTruthy();
  });
});
