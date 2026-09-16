import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { ImpersonationBanner } from "./ImpersonationBanner";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { name?: string }) => (o?.name ? `${k}:${o.name}` : k) }),
}));
const navigate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => navigate }));
const clear = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ clear }) }));
const auth = vi.hoisted(() => ({
  state: { impersonatedBy: undefined as string | undefined, impersonatedUsername: undefined as string | undefined, exitImpersonation: vi.fn() },
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: typeof auth.state) => unknown) => sel(auth.state) }));
vi.mock("@/components/ui/Button", () => ({ Button: ({ children, onClick }: any) => React.createElement("button", { onClick }, children) }));

beforeEach(() => {
  vi.clearAllMocks();
  auth.state.impersonatedBy = undefined;
  auth.state.impersonatedUsername = undefined;
});

// The banner exists only while an admin wears another user's token, names
// that user when known, and Exit restores the admin, drops every response
// cached under the borrowed identity and lands back on the users page.
describe("ImpersonationBanner", () => {
  it("renders nothing outside impersonation", () => {
    const { container } = render(<ImpersonationBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("names the borrowed account or falls back to the generic label", () => {
    auth.state.impersonatedBy = "admin-1";
    auth.state.impersonatedUsername = "bob";
    const { unmount } = render(<ImpersonationBanner />);
    expect(screen.getByText("admin.users.impersonationBanner:bob")).toBeInTheDocument();
    unmount();
    auth.state.impersonatedUsername = undefined;
    render(<ImpersonationBanner />);
    expect(screen.getByText("admin.users.impersonationBannerGeneric")).toBeInTheDocument();
  });

  it("exit restores the admin, clears the cache and returns to the users page", () => {
    auth.state.impersonatedBy = "admin-1";
    render(<ImpersonationBanner />);
    fireEvent.click(screen.getByText("admin.users.exitImpersonation"));
    expect(auth.state.exitImpersonation).toHaveBeenCalledTimes(1);
    expect(clear).toHaveBeenCalledTimes(1);
    expect(navigate).toHaveBeenCalledWith({ to: "/admin/users" });
  });
});
