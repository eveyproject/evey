import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@evey/store", () => ({
  useAuthStore: vi.fn((selector: (s: { profileId: string | null }) => unknown) =>
    selector({ profileId: "profile-1" })
  ),
}));

const mockUseParental = vi.fn(() => ({ data: undefined }));
const mockUseCurrentUser = vi.fn(() => ({ data: { id: "u1" } }));
const mockUseListProfiles = vi.fn(() => ({ data: { profiles: [{ id: "profile-1", region: "US" }] } }));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQuery: vi.fn(() => ({ data: undefined })) };
});

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useParentalServiceGetParentalSettings: (...args: unknown[]) => mockUseParental(...(args as [])),
  useUserServiceGetCurrentUser: (...args: unknown[]) => mockUseCurrentUser(...(args as [])),
  useUserServiceListProfiles: (...args: unknown[]) => mockUseListProfiles(...(args as [])),
}));

describe("unlocked-profile session state", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it("isProfileUnlocked returns false for unknown profile", async () => {
    const { isProfileUnlocked } = await import("./useParentalGate");
    expect(isProfileUnlocked("unknown-profile")).toBe(false);
  });

  it("markUnlocked makes isProfileUnlocked return true", async () => {
    const { isProfileUnlocked, markUnlocked } = await import("./useParentalGate");
    markUnlocked("profile-xyz");
    expect(isProfileUnlocked("profile-xyz")).toBe(true);
  });

  it("marking one profile does not unlock another", async () => {
    const { isProfileUnlocked, markUnlocked } = await import("./useParentalGate");
    markUnlocked("profile-a");
    expect(isProfileUnlocked("profile-b")).toBe(false);
  });
});

describe("useParentalGate hook (age-based)", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.clearAllMocks();
  });

  it("needsPin returns false when parental data is not loaded", async () => {
    mockUseParental.mockReturnValue({ data: undefined });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("needsPin returns false when parental control is disabled", async () => {
    mockUseParental.mockReturnValue({
      // @ts-expect-error test-only partial shape
      data: { enabled: false, hasPin: true, maxAge: 10 },
    });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("needsPin returns true when rating age exceeds max and PIN is set", async () => {
    mockUseParental.mockReturnValue({
      // @ts-expect-error test-only partial shape
      data: { enabled: true, hasPin: true, maxAge: 10 }, // R=17 > 10
    });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(true);
  });

  it("needsPin returns false when profile has been unlocked", async () => {
    mockUseParental.mockReturnValue({
      // @ts-expect-error test-only partial shape
      data: { enabled: true, hasPin: true, maxAge: 10 },
    });
    const { markUnlocked } = await import("./useParentalGate");
    markUnlocked("profile-1");
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("needsPin returns false when rating age does not exceed max", async () => {
    mockUseParental.mockReturnValue({
      // @ts-expect-error test-only partial shape
      data: { enabled: true, hasPin: true, maxAge: 17 },
    });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("PG"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("uses region-specific cert over officialRating when profile.region is set", async () => {
    mockUseListProfiles.mockReturnValue({ data: { profiles: [{ id: "profile-1", region: "FR" }] } });
    mockUseParental.mockReturnValue({
      // @ts-expect-error test-only partial shape
      data: { enabled: true, hasPin: true, maxAge: 10 },
    });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("PG", { FR: "-16", US: "PG" }));
    expect(result.current.needsPin()).toBe(true); // FR -16 > 10
  });
});
