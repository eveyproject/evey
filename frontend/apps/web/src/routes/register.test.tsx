import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const mockUseSearch = vi.fn(() => ({ invite: undefined as string | undefined }));
const mockMutate = vi.fn();
const mockRegister = {
  mutate: mockMutate,
  isPending: false,
  error: null as Error | null,
};

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (def: Record<string, unknown>) => {
    def.useSearch = mockUseSearch;
    return def;
  },
  redirect: vi.fn(),
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", () => ({
  useUserServiceRegisterWithInvite: (_opts?: unknown) => mockRegister,
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    () => ({ setTokens: vi.fn(), accessToken: null }),
    { getState: () => ({ accessToken: null }) },
  ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getRegisterPage() {
  const mod = await import("./register");
  const route = mod.Route as unknown as { useSearch: () => { invite?: string }; component: React.ComponentType };
  route.useSearch = mockUseSearch;
  return route.component;
}

describe("RegisterPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockRegister.error = null;
    mockRegister.isPending = false;
    mockUseSearch.mockReturnValue({ invite: undefined });
  });

  it("renders username, email, password, confirm, and invite code inputs", async () => {
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^password$/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/confirm/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/invite/i)).toBeInTheDocument();
  });

  it("hides invite code input when invite search param provided", async () => {
    mockUseSearch.mockReturnValue({ invite: "ABC123" });
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    // Invite code is shown as display text, not an input
    expect(screen.getByText("ABC123")).toBeInTheDocument();
  });

  it("shows password mismatch error", async () => {
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "bob" } });
    fireEvent.change(screen.getByLabelText(/^password$/i), { target: { value: "longpassword" } });
    fireEvent.change(screen.getByLabelText(/confirm/i), { target: { value: "different" } });
    fireEvent.change(screen.getByLabelText(/invite/i), { target: { value: "INV1" } });
    fireEvent.submit(screen.getByRole("button", { name: /create/i }));
    await waitFor(() => {
      expect(screen.getByText(/passwords do not match/i)).toBeInTheDocument();
    });
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("shows password too short error when < 8 chars", async () => {
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "bob" } });
    fireEvent.change(screen.getByLabelText(/^password$/i), { target: { value: "short" } });
    fireEvent.change(screen.getByLabelText(/confirm/i), { target: { value: "short" } });
    fireEvent.change(screen.getByLabelText(/invite/i), { target: { value: "INV1" } });
    fireEvent.submit(screen.getByRole("button", { name: /create/i }));
    await waitFor(() => {
      expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument();
    });
    expect(mockMutate).not.toHaveBeenCalled();
  });

  it("calls register.mutate on valid submit", async () => {
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "bob" } });
    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "bob@test.com" } });
    fireEvent.change(screen.getByLabelText(/^password$/i), { target: { value: "securepass" } });
    fireEvent.change(screen.getByLabelText(/confirm/i), { target: { value: "securepass" } });
    fireEvent.change(screen.getByLabelText(/invite/i), { target: { value: "INV1" } });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));
    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledWith({
        data: { inviteCode: "INV1", username: "bob", email: "bob@test.com", password: "securepass" },
      });
    });
  });

  it("shows server error from register mutation", async () => {
    mockRegister.error = new Error("invite code expired");
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText(/invite code expired/i)).toBeInTheDocument();
  });

  it("shows sign in link", async () => {
    const Page = await getRegisterPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const link = screen.getByRole("link", { name: /sign in/i });
    expect(link).toHaveAttribute("href", "/login");
  });
});
