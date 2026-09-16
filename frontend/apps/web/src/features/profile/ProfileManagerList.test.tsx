import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, fallback?: string, opts?: { name?: string }) => (typeof fallback === "string" ? fallback.replace("{{name}}", opts?.name ?? "") : _k) }),
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const mut = vi.hoisted(() => ({ del: vi.fn(), setDef: vi.fn(), delOpts: null as any, setDefOpts: null as any }));
vi.mock("@evey/api", () => ({
  useUserServiceDeleteProfile: (opts: any) => {
    mut.delOpts = opts;
    return { mutate: mut.del, isPending: false };
  },
  useUserServiceSetDefaultProfile: (opts: any) => {
    mut.setDefOpts = opts;
    return { mutate: mut.setDef, isPending: false, variables: undefined };
  },
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
  apiErrorMessage: (e: Error, _t: unknown, fallback: string) => e.message || fallback,
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, ...p }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "aria-label": p["aria-label"], title: p.title }, children),
}));
vi.mock("@/components/ui/Badge", () => ({ Badge: ({ children }: any) => React.createElement("span", null, children) }));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: any) => (open ? React.createElement("div", { role: "dialog", "aria-label": title }, children) : null),
}));
vi.mock("./EditProfileDialog", () => ({ EditProfileDialog: ({ open, profile }: any) => (open ? React.createElement("div", null, `editing:${profile?.id}`) : null) }));

import { ProfileManagerList } from "./ProfileManagerList";

const profiles = [
  { id: "p1", name: "Salon", isDefault: true },
  { id: "p2", name: "Kid", isKids: true },
] as any[];

beforeEach(() => vi.clearAllMocks());

// Web twin of the mobile list: the default profile cannot be deleted, a
// delete is confirmed by name, success refreshes the list, and set-as-default
// is gated to the admin flow the RPC belongs to.
describe("ProfileManagerList (web)", () => {
  it("guards the default profile and confirms a delete by name", () => {
    render(<ProfileManagerList userId="u1" profiles={profiles} />);
    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
    const deletes = screen.getAllByRole("button", { name: "Delete" }) as HTMLButtonElement[];
    expect(deletes[0].disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "Set as default" })).toBeNull();

    fireEvent.click(deletes[1]);
    expect(screen.getByRole("dialog")).toHaveTextContent('Delete profile "Kid"? This cannot be undone.');
    fireEvent.click(screen.getByText("Cancel"));
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[1]);
    const confirm = screen.getAllByText("Delete").filter((e) => e.tagName === "BUTTON").at(-1)!;
    fireEvent.click(confirm);
    expect(mut.del).toHaveBeenCalledWith({ id: "p2" });
    mut.delOpts.mutation.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] });
    expect(toast.success).toHaveBeenCalledWith("Profile deleted");
    mut.delOpts.mutation.onError(new Error("kids profile needs a PIN"));
    expect(toast.error).toHaveBeenCalledWith("kids profile needs a PIN");
  });

  it("offers set-as-default on the admin flow only, and opens the editor", () => {
    render(<ProfileManagerList userId="u1" profiles={profiles} canSetDefault />);
    expect(screen.getAllByRole("button", { name: "Set as default" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Set as default" }));
    expect(mut.setDef).toHaveBeenCalledWith({ id: "p2", data: {} });
    mut.setDefOpts.mutation.onSuccess();
    expect(toast.success).toHaveBeenCalledWith("Default profile updated");
    fireEvent.click(screen.getAllByRole("button", { name: "Edit" })[0]);
    expect(screen.getByText("editing:p1")).toBeInTheDocument();
  });
});
