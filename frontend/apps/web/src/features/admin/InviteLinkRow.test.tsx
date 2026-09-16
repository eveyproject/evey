import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { InviteLinkRow } from "./InviteLinkRow";

const revoke = vi.hoisted(() => ({ mutate: vi.fn(), opts: null as any }));
vi.mock("@evey/api", () => ({
  useUserServiceRevokeInviteLink: (opts: any) => {
    revoke.opts = opts;
    return { mutate: revoke.mutate, isPending: false };
  },
  apiErrorMessage: (e: Error, _t: unknown, f: string) => e.message || f,
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("./users-constants", () => ({ INVITES_QUERY_KEY: ["invites"] }));
vi.mock("@/components/ui/Button", () => ({ Button: ({ children, onClick, loading }: any) => React.createElement("button", { onClick, disabled: loading }, children) }));
vi.mock("@/components/ui/Modal", () => ({ Modal: ({ open, children }: any) => (open ? React.createElement("div", { role: "dialog" }, children) : null) }));

function mount(link: Record<string, unknown>) {
  render(<table><tbody><InviteLinkRow link={link as any} /></tbody></table>);
}

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } });
});

// The row builds the registration URL from the code and copies it (saying
// when the clipboard refuses); a revoke is confirmed first and refreshes the
// list once done; uses read "n / max" with an infinity for no cap.
describe("InviteLinkRow", () => {
  it("copies the registration link and confirms a revoke", async () => {
    mount({ id: "i1", code: "ABC123", timesUsed: 2, maxUses: 5, expiresAt: "2026-12-01T00:00:00Z" });
    expect(screen.getByText("ABC123")).toBeInTheDocument();
    expect(screen.getByText("2 / 5")).toBeInTheDocument();
    fireEvent.click(screen.getByText("admin.users.copyLink"));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith(`${window.location.origin}/register?invite=ABC123`));
    expect(toast.success).toHaveBeenCalledWith("admin.users.linkCopied");

    fireEvent.click(screen.getByText("admin.users.revoke"));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent.click(screen.getByText("common.cancel"));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(revoke.mutate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("admin.users.revoke"));
    fireEvent.click(screen.getAllByText("admin.users.revoke").at(-1)!);
    expect(revoke.mutate).toHaveBeenCalledWith({ id: "i1" });
    expect(screen.queryByRole("dialog")).toBeNull();
    revoke.opts.mutation.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["invites"] });
    revoke.opts.mutation.onError(new Error("gone"));
    expect(toast.error).toHaveBeenCalledWith("gone");
  });

  it("reads no cap as infinity, never-expiring as never, and reports a refused clipboard", async () => {
    (navigator.clipboard.writeText as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("denied"));
    mount({ id: "i2", code: "XYZ789" });
    expect(screen.getByText("0 / ∞")).toBeInTheDocument();
    expect(screen.getByText("admin.users.never")).toBeInTheDocument();
    fireEvent.click(screen.getByText("admin.users.copyLink"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("common.clipboardFailed"));
  });
});
