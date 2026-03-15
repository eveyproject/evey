import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetCurrentUser = vi.fn();
const mockUpdateUser = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => mockGetCurrentUser(...args),
  useUserServiceUpdateUser: (...args: unknown[]) => mockUpdateUser(...args),
  getUserServiceGetCurrentUserQueryKey: vi.fn(() => ["user", "me"]),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
  Link: ({ children, to }: { children: React.ReactNode; to: string }) =>
    React.createElement("a", { href: to }, children),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getProfilePage() {
  const mod = await import("./index.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockUser = {
  id: "u1",
  username: "alice",
  email: "alice@example.com",
  isAdmin: false,
  createdAt: "2024-01-15T00:00:00Z",
};

describe("ProfilePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUpdateUser.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("shows loading state", async () => {
    mockGetCurrentUser.mockReturnValue({ data: undefined, isLoading: true });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    expect(screen.getByText(/Loading/i)).toBeInTheDocument();
  });

  it("renders username and email", async () => {
    mockGetCurrentUser.mockReturnValue({ data: mockUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
  });

  it("renders avatar initial", async () => {
    mockGetCurrentUser.mockReturnValue({ data: mockUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    expect(screen.getByText("A")).toBeInTheDocument();
  });

  it("clicking Edit reveals form fields", async () => {
    mockGetCurrentUser.mockReturnValue({ data: mockUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Edit/i }));
    await waitFor(() => expect(screen.getByLabelText(/Username/i)).toBeInTheDocument());
    expect(screen.getByLabelText(/Email/i)).toBeInTheDocument();
  });

  it("clicking Cancel hides form", async () => {
    mockGetCurrentUser.mockReturnValue({ data: mockUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Edit/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /Cancel/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    await waitFor(() => expect(screen.queryByLabelText(/Username/i)).not.toBeInTheDocument());
  });

  it("renders nav links to settings, history, notifications", async () => {
    mockGetCurrentUser.mockReturnValue({ data: mockUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.getByText("Watch History")).toBeInTheDocument();
    expect(screen.getByText("Notifications")).toBeInTheDocument();
  });

  it("shows Admin Panel link for admin users", async () => {
    const adminUser = { ...mockUser, isAdmin: true };
    mockGetCurrentUser.mockReturnValue({ data: adminUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    expect(screen.getByText("Admin Panel")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("does not show Admin Panel link for non-admin users", async () => {
    mockGetCurrentUser.mockReturnValue({ data: mockUser, isLoading: false });
    const ProfilePage = await getProfilePage();
    render(React.createElement(ProfilePage), { wrapper: createWrapper() });
    expect(screen.queryByText("Admin Panel")).not.toBeInTheDocument();
  });
});
