import { renderHook } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockNavigate = vi.fn();
const mockUseAuthStore = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (fn: (s: { accessToken: string | null }) => unknown) => mockUseAuthStore(fn),
}));

async function getUseAuth() {
  const mod = await import("./useAuth");
  return mod.useAuth;
}

describe("useAuth", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns isAuthenticated=true when token present", async () => {
    mockUseAuthStore.mockImplementation((fn: (s: { accessToken: string }) => unknown) =>
      fn({ accessToken: "valid-token" })
    );
    const useAuth = await getUseAuth();
    const { result } = renderHook(() => useAuth());
    expect(result.current.isAuthenticated).toBe(true);
  });

  it("returns isAuthenticated=false when no token", async () => {
    mockUseAuthStore.mockImplementation((fn: (s: { accessToken: null }) => unknown) =>
      fn({ accessToken: null })
    );
    const useAuth = await getUseAuth();
    const { result } = renderHook(() => useAuth());
    expect(result.current.isAuthenticated).toBe(false);
  });

  it("calls navigate to /login when no token", async () => {
    mockUseAuthStore.mockImplementation((fn: (s: { accessToken: null }) => unknown) =>
      fn({ accessToken: null })
    );
    const useAuth = await getUseAuth();
    renderHook(() => useAuth());
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/login" });
  });

  it("does not navigate when token is present", async () => {
    mockUseAuthStore.mockImplementation((fn: (s: { accessToken: string }) => unknown) =>
      fn({ accessToken: "valid-token" })
    );
    const useAuth = await getUseAuth();
    renderHook(() => useAuth());
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});
