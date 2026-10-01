import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SsoSettingsCard, type SsoSettings } from "./SsoSettingsCard";

const fetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({
  eveyFetch: fetchMock,
  apiErrorMessage: (_e: unknown, _t: unknown, fallback: string) => fallback,
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib-1", name: "Movies" }, { id: "lib-2", name: "Kids" }] } }),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@/hooks/useDemoMode", () => ({ useDemoLock: () => ({ demo: false, lock: {} }) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k), i18n: { language: "en" } }),
}));

const MASK = "••••••••";
const settings: SsoSettings = {
  enabled: true,
  issuer: "https://idp.example",
  clientId: "evey",
  clientSecret: MASK,
  allowedEmailDomains: ["example.com"],
  buttonLabel: "",
  linkByEmail: true,
  allowSignup: true,
  allowSignupDefault: true,
  defaultLibraryIds: [],
  redirectUri: "https://evey.example/api/v1/auth/oidc/callback",
  postLogoutRedirectUri: "https://evey.example/login",
  source: "panel",
  status: { state: "error", error: "dial tcp: connection refused", checkedAt: "2026-10-01T10:00:00Z" },
};

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <SsoSettingsCard />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  fetchMock.mockImplementation(async (req: { url: string; method: string; data?: unknown }) => {
    if (req.method === "GET") return settings;
    if (req.url.endsWith("/test")) return { ok: false, error: "no such host" };
    return { ...settings, status: { state: "ok" } };
  });
});

describe("SsoSettingsCard", () => {
  it("shows the provider status, its error and the URIs to register", async () => {
    renderCard();
    expect(await screen.findByTestId("sso-status")).toHaveTextContent("admin.sso.state.error");
    expect(screen.getByTestId("sso-status-error")).toHaveTextContent("connection refused");
    expect(screen.getByText(settings.redirectUri)).toBeInTheDocument();
    expect(screen.getByText(settings.postLogoutRedirectUri)).toBeInTheDocument();
  });

  it("saves every field and sends the mask back untouched for the secret", async () => {
    renderCard();
    await screen.findByTestId("sso-status");
    fireEvent.change(screen.getByLabelText("admin.sso.buttonLabel"), { target: { value: "Corp login" } });
    fireEvent.click(screen.getByTestId("sso-access-selected"));
    expect(screen.getByTestId("sso-save")).toBeDisabled(); // no library chosen yet
    fireEvent.click(screen.getByLabelText("Kids"));
    fireEvent.click(screen.getByTestId("sso-save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.sso.saved"));
    const put = fetchMock.mock.calls.map((c) => c[0]).find((r) => r.method === "PUT");
    expect(put.data).toEqual({
      enabled: true,
      issuer: "https://idp.example",
      clientId: "evey",
      clientSecret: MASK,
      allowedEmailDomains: ["example.com"],
      buttonLabel: "Corp login",
      linkByEmail: true,
      allowSignup: true,
      defaultLibraryIds: ["lib-2"],
    });
  });

  it("reports a failed connection test", async () => {
    renderCard();
    await screen.findByTestId("sso-status");
    fireEvent.click(screen.getByText("admin.sso.test"));
    expect(await screen.findByTestId("sso-test-result")).toHaveTextContent("admin.sso.testFailed(no such host)");
    const post = fetchMock.mock.calls.map((c) => c[0]).find((r) => r.method === "POST");
    expect(post.data).toEqual({ issuer: "https://idp.example" });
  });
});
