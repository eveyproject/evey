import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k),
    i18n: { language: "en" },
  }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888",
      success: "#0f0", danger: "#f00", warning: "#ff0",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useDemoStore: (sel: (s: any) => unknown) => sel({ demoMode: false, demoLocked: [] }), demoLocks: (s: any, k: string) => s.demoMode || s.demoLocked.includes(k) }));

const fetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({
  eveyFetch: fetchMock,
  apiErrorMessage: (_e: unknown, _t: unknown, fallback: string) => fallback,
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib-1", name: "Movies" }, { id: "lib-2", name: "Kids" }] } }),
}));

import { SsoSettingsCard, type SsoSettings } from "./SsoSettingsCard";

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
  source: "env",
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
  fetchMock.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.method === "GET") return settings;
    if (req.url.endsWith("/test")) return { ok: false, error: "no such host" };
    return { ...settings, status: { state: "ok" } };
  });
});

describe("SsoSettingsCard", () => {
  // A failed GET used to leave the spinner turning for good.
  it("says the settings did not load and retries on demand", async () => {
    fetchMock.mockRejectedValueOnce(new Error("offline"));
    renderCard();
    fireEvent.click(await screen.findByLabelText("common.retry"));
    expect(await screen.findByDisplayValue("https://idp.example")).toBeTruthy();
  });

  it("renders the status, its error, the env notice, the fields and the URIs from GET", async () => {
    renderCard();
    expect((await screen.findByTestId("sso-status")).textContent).toBe("admin.sso.state.error");
    expect(screen.getByTestId("sso-status-error").textContent).toContain("connection refused");
    expect(screen.getByText("admin.sso.sourceEnv")).toBeTruthy();
    expect(screen.getByText(/^admin\.sso\.lastChecked\(/)).toBeTruthy();
    expect(screen.getByDisplayValue("https://idp.example")).toBeTruthy();
    expect(screen.getByDisplayValue("evey")).toBeTruthy();
    expect(screen.getByDisplayValue(MASK)).toBeTruthy();
    expect(screen.getByDisplayValue("example.com")).toBeTruthy();
    expect((screen.getByTestId("sso-enabled") as HTMLInputElement).checked).toBe(true);
    expect(screen.getByText(settings.redirectUri)).toBeTruthy();
    expect(screen.getByText(settings.postLogoutRedirectUri)).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/sso", method: "GET" });
  });

  it("saves every field and sends the secret dots back unchanged", async () => {
    renderCard();
    await screen.findByTestId("sso-status");
    fireEvent.change(screen.getByLabelText("admin.sso.buttonLabel"), { target: { value: "Corp login" } });
    fireEvent.click(screen.getByTestId("sso-save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.sso.saved"));
    const put = fetchMock.mock.calls.map((c) => c[0]).find((r) => r.method === "PUT");
    expect(put.url).toBe("/api/v1/admin/sso");
    expect(put.data).toEqual({
      enabled: true,
      issuer: "https://idp.example",
      clientId: "evey",
      clientSecret: MASK,
      allowedEmailDomains: ["example.com"],
      buttonLabel: "Corp login",
      linkByEmail: true,
      allowSignup: true,
      defaultLibraryIds: [],
    });
  });

  it("disables Save with selected libraries until one is chosen, then sends it", async () => {
    renderCard();
    await screen.findByTestId("sso-status");
    fireEvent.click(screen.getByLabelText("admin.sso.defaultAccessSelected"));
    expect((screen.getByTestId("sso-save") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("admin.sso.defaultAccessNone")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Kids"));
    expect((screen.getByTestId("sso-save") as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByTestId("sso-save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    const put = fetchMock.mock.calls.map((c) => c[0]).find((r) => r.method === "PUT");
    expect(put.data.defaultLibraryIds).toEqual(["lib-2"]);
  });

  it("tests the issuer currently typed, not the saved one", async () => {
    renderCard();
    await screen.findByTestId("sso-status");
    fireEvent.change(screen.getByLabelText("admin.sso.issuer"), { target: { value: "https://other.example" } });
    fireEvent.click(screen.getByTestId("sso-test"));
    expect((await screen.findByTestId("sso-test-result")).textContent).toBe("admin.sso.testFailed(no such host)");
    expect(fetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/sso/test", method: "POST", data: { issuer: "https://other.example" } });
  });

  it("gives every setting row exactly one info button", async () => {
    renderCard();
    await screen.findByTestId("sso-status");
    const rows = ["sso-enabled", "sso-issuer", "sso-client-id", "sso-client-secret", "sso-domains", "sso-label", "sso-link", "sso-signup", "sso-default-access"];
    for (const id of rows) {
      const row = screen.getByTestId(`setting-row-${id}`);
      expect(within(row).getAllByTestId("setting-info-button")).toHaveLength(1);
    }
    expect(screen.getAllByTestId("setting-info-button")).toHaveLength(rows.length);
  });
});
