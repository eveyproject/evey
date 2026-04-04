import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockListUsers = vi.fn();
const mockListInviteLinks = vi.fn();
const mockCreateUser = vi.fn();
const mockDeleteUser = vi.fn();
const mockDisableUser = vi.fn();
const mockUpdateUser = vi.fn();
const mockCreateInviteLink = vi.fn();
const mockRevokeInviteLink = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceListUsers: (...args: unknown[]) => mockListUsers(...args),
  useUserServiceCreateUser: (...args: unknown[]) => mockCreateUser(...args),
  useUserServiceDeleteUser: (...args: unknown[]) => mockDeleteUser(...args),
  useUserServiceDisableUser: (...args: unknown[]) => mockDisableUser(...args),
  useUserServiceUpdateUser: (...args: unknown[]) => mockUpdateUser(...args),
  useUserServiceListInviteLinks: (...args: unknown[]) => mockListInviteLinks(...args),
  useUserServiceCreateInviteLink: (...args: unknown[]) => mockCreateInviteLink(...args),
  useUserServiceRevokeInviteLink: (...args: unknown[]) => mockRevokeInviteLink(...args),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
    useQuery: vi.fn(() => ({ data: undefined, isLoading: false })),
  };
});

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getUsersPage() {
  const mod = await import("./users.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockUsers = [
  { id: "u1", username: "alice", email: "alice@example.com", isAdmin: false, enabled: true },
  { id: "u2", username: "bob", email: "bob@example.com", isAdmin: true, enabled: true },
];

const mockInvites = [
  { id: "inv1", code: "INVITE123", timesUsed: 1, maxUses: 5, expiresAt: "2025-12-31T00:00:00Z" },
];

describe("UsersPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const mutate = vi.fn();
    mockListUsers.mockReturnValue({ data: { users: [] }, isLoading: false });
    mockListInviteLinks.mockReturnValue({ data: { inviteLinks: [] }, isLoading: false });
    mockCreateUser.mockReturnValue({ mutate, isPending: false });
    mockDeleteUser.mockReturnValue({ mutate, isPending: false });
    mockDisableUser.mockReturnValue({ mutate, isPending: false });
    mockUpdateUser.mockReturnValue({ mutate, isPending: false });
    mockCreateInviteLink.mockReturnValue({ mutate, isPending: false });
    mockRevokeInviteLink.mockReturnValue({ mutate, isPending: false });
  });

  it("renders Users and Invite Links sections", async () => {
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    expect(screen.getByText("Users")).toBeInTheDocument();
    expect(screen.getByText("Invite Links")).toBeInTheDocument();
  });

  it("shows empty state when no users", async () => {
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    expect(screen.getByText(/No users found/i)).toBeInTheDocument();
  });

  it("renders user rows with username and role badge", async () => {
    mockListUsers.mockReturnValue({ data: { users: mockUsers }, isLoading: false });
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("bob")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("clicking Create User button opens modal", async () => {
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Create User/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByLabelText(/Username/i)).toBeInTheDocument();
  });

  it("clicking Create Invite Link button opens modal", async () => {
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Create Invite Link/i }));
    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
      expect(screen.getByText(/Max uses/i)).toBeInTheDocument();
    });
  });

  it("renders invite link rows with code", async () => {
    mockListInviteLinks.mockReturnValue({ data: { inviteLinks: mockInvites }, isLoading: false });
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    expect(screen.getByText("INVITE123")).toBeInTheDocument();
  });

  it("non-admin user has Libraries and Delete buttons", async () => {
    mockListUsers.mockReturnValue({ data: { users: [mockUsers[0]] }, isLoading: false });
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Libraries/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Delete/i })).toBeInTheDocument();
  });

  it("admin user does not have Libraries button", async () => {
    mockListUsers.mockReturnValue({ data: { users: [mockUsers[1]] }, isLoading: false });
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    expect(screen.queryByRole("button", { name: /Libraries/i })).not.toBeInTheDocument();
  });

  it("clicking Delete opens confirmation modal", async () => {
    mockListUsers.mockReturnValue({ data: { users: [mockUsers[0]] }, isLoading: false });
    const UsersPage = await getUsersPage();
    render(React.createElement(UsersPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Delete/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText(/Delete user "alice"/i)).toBeInTheDocument();
  });
});
