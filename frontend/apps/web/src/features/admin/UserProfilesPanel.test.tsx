import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { UserProfilesPanel } from "./UserProfilesPanel";

const mockImpersonate = vi.fn();
const mockNavigate = vi.fn();
const mockClear = vi.fn();
const mockDoImpersonate = vi.fn();
let impersonateOnSuccess: ((d: { accessToken?: string }) => void) | undefined;

vi.mock("@tanstack/react-query", async (importActual) => ({
  ...(await importActual<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ clear: mockClear }),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceImpersonateUser: (opts: {
    mutation?: { onSuccess?: (d: { accessToken?: string }) => void };
  }) => {
    impersonateOnSuccess = opts?.mutation?.onSuccess;
    return { mutate: mockImpersonate, isPending: false };
  },
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel?: (s: Record<string, unknown>) => unknown) => {
    const state = { impersonate: mockDoImpersonate };
    return sel ? sel(state) : state;
  },
}));

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

const user = { id: "usr-1", username: "alice", enabled: true };
const profiles = [
  { id: "prf-1", userId: "usr-1", name: "Papa", isDefault: true },
  { id: "prf-2", userId: "usr-1", name: "Les enfants", isKids: true },
];

describe("UserProfilesPanel", () => {
  beforeEach(() => vi.clearAllMocks());

  it("names every profile of the account", () => {
    render(<UserProfilesPanel user={user} profiles={profiles} />);
    expect(screen.getByText("Papa")).toBeInTheDocument();
    expect(screen.getByText("Les enfants")).toBeInTheDocument();
  });

  // The gap this panel exists to close: impersonation used to be per USER and
  // always landed on the default profile.
  it("impersonates the profile that was clicked, not the default one", () => {
    render(<UserProfilesPanel user={user} profiles={profiles} />);
    fireEvent.click(screen.getByRole("button", { name: /Les enfants/ }));
    expect(mockImpersonate).toHaveBeenCalledWith({
      userId: "usr-1",
      data: { profileId: "prf-2" },
    });
  });

  it("clears the query cache on handover so no admin-scoped data leaks in", () => {
    render(<UserProfilesPanel user={user} profiles={profiles} />);
    impersonateOnSuccess?.({ accessToken: "tok" });
    expect(mockDoImpersonate).toHaveBeenCalledWith("tok");
    expect(mockClear).toHaveBeenCalled();
  });

  it("deep-links the history to that one profile", () => {
    render(<UserProfilesPanel user={user} profiles={profiles} />);
    const [historyBtn] = screen.getAllByRole("button", { name: /History|Historique/ });
    fireEvent.click(historyBtn);
    expect(mockNavigate).toHaveBeenCalledWith({
      to: "/admin/activity",
      search: { profileId: "prf-1" },
    });
  });

  // "When was this profile last used?" is what the expanded row is for; a
  // profile nobody ever opened must say so rather than show a blank.
  it("shows when each profile was last active, or never", () => {
    const at = "2026-09-01T10:00:00Z";
    render(
      <UserProfilesPanel
        user={user}
        profiles={[{ ...profiles[0], lastActiveAt: at }, profiles[1]]}
      />,
    );
    expect(screen.getByText(new RegExp(new Date(at).getFullYear().toString()))).toBeInTheDocument();
    expect(screen.getByText(/Never|Jamais/)).toBeInTheDocument();
  });

  it("offers no impersonation for a disabled account", () => {
    render(<UserProfilesPanel user={{ ...user, enabled: false }} profiles={profiles} />);
    expect(screen.queryByRole("button", { name: /Papa/ })).not.toBeInTheDocument();
  });
});
