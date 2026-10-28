import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { EditUserModal } from "./EditUserModal";

const updateMutateAsync = vi.fn();
const setAccessMutateAsync = vi.fn();
const revokeAccessMutateAsync = vi.fn();
const getAccessMock = vi.fn();
const currentUserMock = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  // The mock propagates onError so tests can assert that only ONE toast fires
  // when the mutation rejects (the real bug was: onError on the hook + catch = 2 toasts).
  useUserServiceUpdateUser: (opts?: { mutation?: { onError?: (err: Error) => void } }) => ({
    mutateAsync: async (...args: Parameters<typeof updateMutateAsync>) => {
      try {
        return await updateMutateAsync(...args);
      } catch (err) {
        opts?.mutation?.onError?.(err as Error);
        throw err;
      }
    },
    isPending: false,
  }),
  useUserServiceListProfiles: (_id: string, _opts: unknown) => ({ data: { profiles: [] }, isLoading: false }),
  useUserServiceGetCurrentUser: () => currentUserMock(),
  useUserServiceCreateProfile: () => ({ mutate: vi.fn(), isPending: false }),
  useUserServiceDeleteProfile: () => ({ mutate: vi.fn(), isPending: false }),
  useUserServiceSetDefaultProfile: () => ({ mutate: vi.fn(), isPending: false }),
  useLibraryAccessServiceGetUserLibraryAccess: (_id: string, _opts: unknown) => getAccessMock(),
  useLibraryAccessServiceSetUserLibraryAccess: () => ({ mutateAsync: setAccessMutateAsync, isPending: false }),
  useLibraryAccessServiceRevokeAllUserLibraryAccess: () => ({ mutateAsync: revokeAccessMutateAsync, isPending: false }),
  getLibraryAccessServiceGetUserLibraryAccessQueryKey: (id: string) => ["library-access", id],
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
  getUserServiceListUsersQueryKey: () => ["/api/v1/users"],
  getUserServiceListInviteLinksQueryKey: () => ["/api/v1/invites"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const baseUser = {
  id: "u1",
  username: "alice",
  email: "alice@example.com",
  isAdmin: false,
};

describe("EditUserModal", () => {
  beforeEach(() => {
    updateMutateAsync.mockReset().mockResolvedValue({});
    setAccessMutateAsync.mockReset().mockResolvedValue({});
    revokeAccessMutateAsync.mockReset().mockResolvedValue({});
    currentUserMock.mockReset().mockReturnValue({ data: { id: "admin-1" } });
    getAccessMock.mockReset().mockReturnValue({
      data: { libraryIds: [], libraries: [] },
      isLoading: false,
    });
  });

  it("renders nothing when closed", () => {
    render(<EditUserModal user={baseUser} open={false} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("pre-fills username/email/isAdmin from user prop", () => {
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByLabelText("Username")).toHaveValue("alice");
    expect(screen.getByLabelText("Email")).toHaveValue("alice@example.com");
    expect(screen.getByLabelText("Admin")).not.toBeChecked();
  });

  it("submits only changed email field", async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    render(<EditUserModal user={baseUser} open={true} onClose={onClose} />, { wrapper: createWrapper() });
    const email = screen.getByLabelText("Email");
    await u.clear(email);
    await u.type(email, "new@example.com");
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(updateMutateAsync).toHaveBeenCalledWith({
      id: "u1",
      data: { email: "new@example.com" },
    });
    expect(onClose).toHaveBeenCalled();
  });

  it("submits isAdmin toggle", async () => {
    const u = userEvent.setup();
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    await u.click(screen.getByLabelText("Admin"));
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(updateMutateAsync).toHaveBeenCalledWith({
      id: "u1",
      data: { isAdmin: true },
    });
  });

  it("does not call mutate when nothing changed", async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    render(<EditUserModal user={baseUser} open={true} onClose={onClose} />, { wrapper: createWrapper() });
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(updateMutateAsync).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("Cancel calls onClose without mutation", async () => {
    const u = userEvent.setup();
    const onClose = vi.fn();
    render(<EditUserModal user={baseUser} open={true} onClose={onClose} />, { wrapper: createWrapper() });
    await u.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
    expect(updateMutateAsync).not.toHaveBeenCalled();
  });

  it("shows exactly one error toast when mutation fails", async () => {
    // This test guards against the double-toast regression where onError on the
    // hook AND the catch block both called toast.error on the same failure.
    const { toast } = await import("@evey/store");
    updateMutateAsync.mockRejectedValueOnce(new Error("Server error"));
    const u = userEvent.setup();
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    // Make a change so the mutation fires
    const email = screen.getByLabelText("Email");
    await u.clear(email);
    await u.type(email, "changed@example.com");
    await u.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(toast.error).toHaveBeenCalledTimes(1);
  });

  // AUTH-3: the admin editing their OWN account proves it with their password;
  // editing someone else (every test above) never asks for it.
  it("asks an admin editing their own account for the current password", async () => {
    currentUserMock.mockReturnValue({ data: { id: "u1" } });
    const u = userEvent.setup();
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    const email = screen.getByLabelText("Email");
    await u.clear(email);
    await u.type(email, "self@example.com");
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(updateMutateAsync).not.toHaveBeenCalled();

    await u.type(screen.getByLabelText("Current password"), "mypassword");
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(updateMutateAsync).toHaveBeenCalledWith({
      id: "u1",
      data: { email: "self@example.com", currentPassword: "mypassword" },
    });
  });

  it("does not show the current password field for another user", () => {
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.queryByLabelText("Current password")).not.toBeInTheDocument();
  });

  it("unticking the last library confines the user to none, never to every library", async () => {
    // An empty selection used to be saved as a revoke-all, which lifts the
    // restriction and opens the whole catalogue.
    getAccessMock.mockReturnValue({
      data: { libraryIds: ["lib1"], restricted: true, libraries: [{ id: "lib1", name: "Movies" }] },
      isLoading: false,
    });
    const u = userEvent.setup();
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    await u.click(screen.getByLabelText("Movies"));
    expect(screen.getByText("No library")).toBeInTheDocument();
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(setAccessMutateAsync).toHaveBeenCalledWith({ userId: "u1", data: { libraryIds: [] } });
    expect(revokeAccessMutateAsync).not.toHaveBeenCalled();
  });

  it("only the explicit All libraries choice lifts the restriction", async () => {
    getAccessMock.mockReturnValue({
      data: { libraryIds: [], restricted: true, libraries: [{ id: "lib1", name: "Movies" }] },
      isLoading: false,
    });
    const u = userEvent.setup();
    render(<EditUserModal user={baseUser} open={true} onClose={vi.fn()} />, { wrapper: createWrapper() });
    expect(screen.getByText("No library")).toBeInTheDocument();
    await u.click(screen.getByRole("radio", { name: "All libraries" }));
    await u.click(screen.getByRole("button", { name: "Save" }));
    expect(revokeAccessMutateAsync).toHaveBeenCalledWith({ userId: "u1" });
    expect(setAccessMutateAsync).not.toHaveBeenCalled();
  });
});
