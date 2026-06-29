import React from "react";
import { render } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockRequestReset = { mutate: vi.fn(), isPending: false };
// Mutable so each test can flip whether the server has recovery enabled.
let mockSetupData: { passwordRecoveryEnabled?: boolean } | undefined = undefined;

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (def: Record<string, unknown>) => def,
  redirect: vi.fn(),
  Link: ({ children }: { children: React.ReactNode }) =>
    React.createElement("a", null, children),
}));

vi.mock("@evey/api", () => ({
  useUserServiceRequestPasswordReset: (_opts?: unknown) => mockRequestReset,
  useAdminSystemServiceGetSetupStatus: () => ({ data: mockSetupData }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    () => ({ accessToken: null }),
    { getState: () => ({ accessToken: null }) },
  ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function renderPage() {
  const mod = await import("./forgot-password");
  const route = mod.Route as unknown as { component: React.ComponentType };
  const Page = route.component;
  return render(React.createElement(Page), { wrapper: createWrapper() });
}

describe("ForgotPasswordPage", () => {
  beforeEach(() => {
    mockSetupData = undefined;
    mockRequestReset.mutate.mockReset();
  });

  it("shows the email form while setup status is loading", async () => {
    mockSetupData = undefined;
    const { container } = await renderPage();
    expect(container.querySelector('input[type="email"]')).not.toBeNull();
  });

  it("shows the email form when password recovery is enabled", async () => {
    mockSetupData = { passwordRecoveryEnabled: true };
    const { container } = await renderPage();
    expect(container.querySelector('input[type="email"]')).not.toBeNull();
  });

  it("hides the form (honest message instead) when recovery is disabled", async () => {
    mockSetupData = { passwordRecoveryEnabled: false };
    const { container } = await renderPage();
    // No form to submit — the page must not promise a reset email that will
    // never be sent when SMTP/recovery is off.
    expect(container.querySelector('input[type="email"]')).toBeNull();
    expect(container.querySelector("form")).toBeNull();
  });
});
