import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const mockUseSearch = vi.fn(() => ({ oidc_error: undefined }));
const mockMutate = vi.fn();
const mockLogin = {
  mutate: mockMutate,
  isPending: false,
  error: null as Error | null,
};
const mockUseQuery = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (def: Record<string, unknown>) => {
    def.useSearch = mockUseSearch;
    return def;
  },
  redirect: vi.fn(),
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceLogin: (_opts?: unknown) => mockLogin,
  useAdminSystemServiceGetSetupStatus: () => ({ data: { passwordRecoveryEnabled: false } }),
  // Resolves to an empty payload: a bare vi.fn() hands react-query
  // `undefined`, which it rejects with a warning on every render.
  eveyFetch: vi.fn().mockResolvedValue({}),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    () => ({ setTokens: vi.fn(), accessToken: null }),
    { getState: () => ({ accessToken: null }) },
  ),
}));

vi.mock("@/components/QuickConnect", () => ({
  QuickConnect: () => React.createElement("div", { "data-testid": "quick-connect" }, "QuickConnect"),
}));

vi.mock("@/features/plugins/PluginSlot", () => ({
  PluginSlot: () => null,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getLoginPage() {
  const mod = await import("./login");
  const route = mod.Route as unknown as { useSearch: () => { oidc_error?: string }; component: React.ComponentType };
  route.useSearch = mockUseSearch;
  return route.component;
}

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockLogin.error = null;
    mockLogin.isPending = false;
    mockUseSearch.mockReturnValue({ oidc_error: undefined });
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: false });
  });

  it("renders username and password inputs", async () => {
    const LoginPage = await getLoginPage();
    render(React.createElement(LoginPage), { wrapper: createWrapper() });
    expect(screen.getByLabelText(/username/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i, { selector: "input" })).toBeInTheDocument();
  });

  it("renders sign in button", async () => {
    const LoginPage = await getLoginPage();
    render(React.createElement(LoginPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("login-submit")).toBeInTheDocument();
  });

  it("calls login.mutate on form submit", async () => {
    const LoginPage = await getLoginPage();
    render(React.createElement(LoginPage), { wrapper: createWrapper() });
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "alice" } });
    fireEvent.change(screen.getByLabelText(/password/i, { selector: "input" }), { target: { value: "secret123" } });
    fireEvent.click(screen.getByTestId("login-submit"));
    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledWith({ data: { username: "alice", password: "secret123" } });
    });
  });

  it("shows error on failed login", async () => {
    mockLogin.error = new Error("invalid credentials");
    const LoginPage = await getLoginPage();
    render(React.createElement(LoginPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("login-error")).toBeInTheDocument();
  });

  it("shows SSO failed message when oidc_error search param present", async () => {
    mockUseSearch.mockReturnValue({ oidc_error: "some_error" });
    const LoginPage = await getLoginPage();
    render(React.createElement(LoginPage), { wrapper: createWrapper() });
    expect(screen.getByText(/sso/i)).toBeInTheDocument();
  });

  it("renders QuickConnect section", async () => {
    const LoginPage = await getLoginPage();
    render(React.createElement(LoginPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("quick-connect")).toBeInTheDocument();
  });
});
