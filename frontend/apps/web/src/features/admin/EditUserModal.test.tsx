import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { EditUserModal } from "./EditUserModal";

const updateMutateAsync = vi.fn();
const setAccessMutateAsync = vi.fn();
const revokeAccessMutateAsync = vi.fn();
const getAccessMock = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceUpdateUser: (_opts: unknown) => ({ mutateAsync: updateMutateAsync, isPending: false }),
  useLibraryAccessServiceGetUserLibraryAccess: (_id: string, _opts: unknown) => getAccessMock(),
  useLibraryAccessServiceSetUserLibraryAccess: () => ({ mutateAsync: setAccessMutateAsync, isPending: false }),
  useLibraryAccessServiceRevokeAllUserLibraryAccess: () => ({ mutateAsync: revokeAccessMutateAsync, isPending: false }),
  getLibraryAccessServiceGetUserLibraryAccessQueryKey: (id: string) => ["library-access", id],
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
});
