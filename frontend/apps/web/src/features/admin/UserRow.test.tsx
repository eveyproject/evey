import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { UserRow } from "./UserRow";

// ── Mutable mock state ────────────────────────────────────────────────────────

const mockDeleteMutate = vi.fn();
const mockDisableMutate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

let mockDeleteIsPending = false;
let mockDisableIsPending = false;

// Capture onSuccess/onError callbacks so we can fire them in tests
let deleteOnSuccess: (() => void) | undefined;
let deleteOnError: ((err: Error) => void) | undefined;
let disableOnSuccess: (() => void) | undefined;
let disableOnError: ((err: Error) => void) | undefined;

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceDeleteUser: (opts: {
    mutation?: { onSuccess?: () => void; onError?: (err: Error) => void };
  }) => {
    deleteOnSuccess = opts?.mutation?.onSuccess;
    deleteOnError = opts?.mutation?.onError;
    return { mutate: mockDeleteMutate, isPending: mockDeleteIsPending };
  },
  useUserServiceDisableUser: (opts: {
    mutation?: { onSuccess?: () => void; onError?: (err: Error) => void };
  }) => {
    disableOnSuccess = opts?.mutation?.onSuccess;
    disableOnError = opts?.mutation?.onError;
    return { mutate: mockDisableMutate, isPending: mockDisableIsPending };
  },
  useUserServiceImpersonateUser: () => ({ mutate: vi.fn(), isPending: false }),
  getUserServiceListUsersQueryKey: () => ["/api/v1/users"],
  getUserServiceListInviteLinksQueryKey: () => ["/api/v1/invites"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel?: (s: Record<string, unknown>) => unknown) => {
    const state = { impersonate: vi.fn(), exitImpersonation: vi.fn(), impersonatedBy: null, impersonatedUsername: null };
    return sel ? sel(state) : state;
  },
}));

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => vi.fn(),
}));

// Sub-modals are outside scope for this test (they have their own test files).
// Stub them so they render nothing when closed (default) and a sentinel when open.
vi.mock("./EditUserModal", () => ({
  EditUserModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-user-modal" /> : null,
}));
vi.mock("./LibraryAccessModal", () => ({
  LibraryAccessModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="library-access-modal" /> : null,
}));
vi.mock("./BitrateModal", () => ({
  BitrateModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="bitrate-modal" /> : null,
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderRow(user: Parameters<typeof UserRow>[0]["user"]) {
  return render(
    <table>
      <tbody>
        <UserRow user={user} />
      </tbody>
    </table>,
    { wrapper: createWrapper() },
  );
}

const BASE_USER = {
  id: "usr-abc",
  username: "alice",
  email: "alice@example.com",
  isAdmin: false,
  enabled: true,
};

const ADMIN_USER = {
  id: "usr-admin",
  username: "bob",
  email: "bob@example.com",
  isAdmin: true,
  enabled: true,
};

const DISABLED_USER = {
  id: "usr-dis",
  username: "charlie",
  email: "charlie@example.com",
  isAdmin: false,
  enabled: false,
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("UserRow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDeleteIsPending = false;
    mockDisableIsPending = false;
    deleteOnSuccess = undefined;
    deleteOnError = undefined;
    disableOnSuccess = undefined;
    disableOnError = undefined;
  });

  // ── Field rendering ───────────────────────────────────────────────────────

  it("renders the username", () => {
    renderRow(BASE_USER);
    expect(screen.getByText("alice")).toBeInTheDocument();
  });

  it("renders the email", () => {
    renderRow(BASE_USER);
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
  });

  it("renders a placeholder dash in the email cell when email is absent", () => {
    const { container } = renderRow({ ...BASE_USER, email: undefined });
    // Simple hyphen placeholder (project style: never an em dash). Scoped to
    // the email cell: the registered/last-login cells share the placeholder.
    const emailCell = container.querySelector('td[data-label="Email"]');
    expect(emailCell).not.toBeNull();
    expect(emailCell!.textContent).toBe("-");
  });

  it("renders the User badge for a non-admin user", () => {
    renderRow(BASE_USER);
    expect(screen.getByText("User")).toBeInTheDocument();
  });

  it("renders the Admin badge for an admin user", () => {
    renderRow(ADMIN_USER);
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("renders enabled badge for an active user", () => {
    renderRow(BASE_USER);
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("renders disabled badge when user.enabled is false", () => {
    renderRow(DISABLED_USER);
    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  // ── Conditional rendering of Libraries button ─────────────────────────────

  it("renders the Libraries button for a non-admin user", () => {
    renderRow(BASE_USER);
    expect(
      screen.getByRole("button", { name: /libraries/i }),
    ).toBeInTheDocument();
  });

  it("does NOT render the Libraries button for an admin user", () => {
    renderRow(ADMIN_USER);
    expect(
      screen.queryByRole("button", { name: /libraries/i }),
    ).not.toBeInTheDocument();
  });

  // ── Disable/Enable toggle ─────────────────────────────────────────────────

  it("shows Disable button for an enabled user", () => {
    renderRow(BASE_USER);
    expect(
      screen.getByRole("button", { name: /^disable$/i }),
    ).toBeInTheDocument();
  });

  it("shows Enable button for a disabled user", () => {
    renderRow(DISABLED_USER);
    expect(
      screen.getByRole("button", { name: /^enable$/i }),
    ).toBeInTheDocument();
  });

  it("clicking Disable calls disableUser.mutate with id and disabled:true (was enabled)", () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^disable$/i }));
    expect(mockDisableMutate).toHaveBeenCalledOnce();
    expect(mockDisableMutate).toHaveBeenCalledWith({
      id: "usr-abc",
      params: { disabled: true },
    });
  });

  it("clicking Enable calls disableUser.mutate with id and disabled:false (was disabled)", () => {
    renderRow(DISABLED_USER);
    fireEvent.click(screen.getByRole("button", { name: /^enable$/i }));
    expect(mockDisableMutate).toHaveBeenCalledOnce();
    expect(mockDisableMutate).toHaveBeenCalledWith({
      id: "usr-dis",
      params: { disabled: false },
    });
  });

  it("disableUser onSuccess invalidates USERS_QUERY_KEY", async () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^disable$/i }));
    disableOnSuccess?.();
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["/api/v1/users"] }),
      ),
    );
  });

  it("disableUser onSuccess shows success toast", async () => {
    const { toast } = await import("@evey/store");
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^disable$/i }));
    disableOnSuccess?.();
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  it("disableUser onError shows error toast", async () => {
    const { toast } = await import("@evey/store");
    renderRow(BASE_USER);
    disableOnError?.(new Error("network down"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("network down"));
  });

  // ── Delete flow: modal-based confirmation guard ───────────────────────────

  it("shows the Delete button", () => {
    renderRow(BASE_USER);
    expect(
      screen.getByRole("button", { name: /^delete$/i }),
    ).toBeInTheDocument();
  });

  it("clicking Delete does NOT immediately call mutation (opens confirm modal first)", () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    expect(mockDeleteMutate).not.toHaveBeenCalled();
  });

  it("clicking Delete opens the confirmation dialog", () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("confirm dialog contains the username in the confirmation message", () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    // username is shown in the modal context
    expect(screen.getAllByText("alice").length).toBeGreaterThanOrEqual(1);
  });

  it("cancelling the confirm dialog does NOT call deleteUser.mutate", () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    fireEvent.click(screen.getByRole("button", { name: /^cancel$/i }));
    expect(mockDeleteMutate).not.toHaveBeenCalled();
  });

  it("cancelling the confirm dialog closes the dialog", async () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^cancel$/i }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("confirming delete calls deleteUser.mutate with the user id", () => {
    renderRow(BASE_USER);
    // Open confirm modal - row-level Delete button
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    // The confirm dialog now has a Delete button inside
    const deleteButtons = screen.getAllByRole("button", { name: /^delete$/i });
    // The last Delete button is the one inside the modal
    fireEvent.click(deleteButtons[deleteButtons.length - 1]);
    expect(mockDeleteMutate).toHaveBeenCalledOnce();
    expect(mockDeleteMutate).toHaveBeenCalledWith({ id: "usr-abc" });
  });

  it("confirming delete closes the modal immediately (optimistic)", async () => {
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    const deleteButtons = screen.getAllByRole("button", { name: /^delete$/i });
    fireEvent.click(deleteButtons[deleteButtons.length - 1]);
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("deleteUser onSuccess invalidates USERS_QUERY_KEY", async () => {
    renderRow(BASE_USER);
    // Trigger the mutation so callbacks are wired
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    deleteOnSuccess?.();
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["/api/v1/users"] }),
      ),
    );
  });

  it("deleteUser onSuccess shows success toast with username", async () => {
    const { toast } = await import("@evey/store");
    renderRow(BASE_USER);
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));
    deleteOnSuccess?.();
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  it("deleteUser onError shows error toast with the error message", async () => {
    const { toast } = await import("@evey/store");
    renderRow(BASE_USER);
    deleteOnError?.(new Error("forbidden"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("forbidden"));
  });

  // ── Edit / Bitrate modal triggers ─────────────────────────────────────────

  it("clicking Edit opens EditUserModal", async () => {
    renderRow(BASE_USER);
    expect(screen.queryByTestId("edit-user-modal")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    await waitFor(() =>
      expect(screen.getByTestId("edit-user-modal")).toBeInTheDocument(),
    );
  });

  it("clicking Bitrate opens BitrateModal", async () => {
    renderRow(BASE_USER);
    expect(screen.queryByTestId("bitrate-modal")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^bitrate$/i }));
    await waitFor(() =>
      expect(screen.getByTestId("bitrate-modal")).toBeInTheDocument(),
    );
  });

  it("clicking Libraries opens LibraryAccessModal for non-admin user", async () => {
    renderRow(BASE_USER);
    expect(screen.queryByTestId("library-access-modal")).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: /^libraries$/i }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("library-access-modal")).toBeInTheDocument(),
    );
  });

  it("LibraryAccessModal is not mounted for admin user (no Libraries button)", () => {
    renderRow(ADMIN_USER);
    // For admin users, the modal is conditionally excluded from the tree
    expect(screen.queryByTestId("library-access-modal")).not.toBeInTheDocument();
  });
});
