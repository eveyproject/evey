import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockNavigate = vi.fn();
const mockSetTokens = vi.fn();
const mockEveyFetch = vi.fn();
const mockUseSearch = vi.fn(() => ({ oidc_error: undefined }));

vi.mock("@tanstack/react-router", () => ({
  createFileRoute: () => (def: Record<string, unknown>) => {
    def.useSearch = mockUseSearch;
    return def;
  },
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: () => ({ setTokens: mockSetTokens }),
}));

async function getOIDCCallbackPage() {
  const mod = await import("./oidc-callback");
  const route = mod.Route as unknown as { useSearch: () => { oidc_error?: string }; component: React.ComponentType };
  route.useSearch = mockUseSearch;
  return route.component;
}

describe("OIDCCallbackPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseSearch.mockReturnValue({ oidc_error: undefined });
  });

  it("renders signing-in message", async () => {
    mockEveyFetch.mockReturnValue(new Promise(() => {})); // never resolves
    const Page = await getOIDCCallbackPage();
    render(React.createElement(Page));
    expect(screen.getByText(/signing/i)).toBeInTheDocument();
  });

  it("redirects to /login carrying the reason when an error is present", async () => {
    mockUseSearch.mockReturnValue({ oidc_error: "access_denied" });
    const Page = await getOIDCCallbackPage();
    render(React.createElement(Page));
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith({ to: "/login", search: { oidc_error: "access_denied" } });
    });
    expect(mockEveyFetch).not.toHaveBeenCalled();
  });

  it("fetches session and navigates to /profile-select on success", async () => {
    mockEveyFetch.mockResolvedValue({ accessToken: "tok-123" });
    const Page = await getOIDCCallbackPage();
    render(React.createElement(Page));
    await waitFor(() => {
      expect(mockSetTokens).toHaveBeenCalledWith("tok-123");
    });
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/profile-select" });
  });

  it("navigates to /login on fetch failure", async () => {
    mockEveyFetch.mockRejectedValue(new Error("network"));
    const Page = await getOIDCCallbackPage();
    render(React.createElement(Page));
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith({ to: "/login" });
    });
    expect(mockSetTokens).not.toHaveBeenCalled();
  });
});
