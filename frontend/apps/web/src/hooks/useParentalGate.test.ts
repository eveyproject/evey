import { describe, it, expect, beforeEach, vi } from "vitest";

// sessionStorage is reset between test suites
vi.mock("@evey/store", () => ({
  useAuthStore: vi.fn((selector: (s: { profileId: string | null }) => unknown) =>
    selector({ profileId: "profile-1" })
  ),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQuery: vi.fn(() => ({ data: undefined })) };
});

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

describe("isRatingExceeded helpers (via useParentalGate exports)", () => {
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

  it("markUnlocked is idempotent", async () => {
    const { isProfileUnlocked, markUnlocked } = await import("./useParentalGate");
    markUnlocked("profile-idem");
    markUnlocked("profile-idem");
    expect(isProfileUnlocked("profile-idem")).toBe(true);
  });

  it("sessionStorage persists across markUnlocked calls", async () => {
    const { isProfileUnlocked, markUnlocked } = await import("./useParentalGate");
    markUnlocked("p1");
    markUnlocked("p2");
    expect(isProfileUnlocked("p1")).toBe(true);
    expect(isProfileUnlocked("p2")).toBe(true);
  });
});

describe("useParentalGate hook", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.clearAllMocks();
  });

  it("needsPin returns false when parental data is not loaded", async () => {
    const { useQuery } = await import("@tanstack/react-query");
    vi.mocked(useQuery).mockReturnValue({ data: undefined } as ReturnType<typeof useQuery>);
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("needsPin returns false when parental control is disabled", async () => {
    const { useQuery } = await import("@tanstack/react-query");
    vi.mocked(useQuery).mockReturnValue({
      data: { enabled: false, hasPin: true, maxRating: "PG" },
    } as ReturnType<typeof useQuery>);
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("needsPin returns true when rating exceeds max and PIN is set", async () => {
    const { useQuery } = await import("@tanstack/react-query");
    vi.mocked(useQuery).mockReturnValue({
      data: { enabled: true, hasPin: true, maxRating: "PG" },
    } as ReturnType<typeof useQuery>);
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R")); // R > PG
    expect(result.current.needsPin()).toBe(true);
  });

  it("needsPin returns false when profile has been unlocked", async () => {
    const { useQuery } = await import("@tanstack/react-query");
    vi.mocked(useQuery).mockReturnValue({
      data: { enabled: true, hasPin: true, maxRating: "PG" },
    } as ReturnType<typeof useQuery>);
    const { markUnlocked } = await import("./useParentalGate");
    markUnlocked("profile-1"); // profile-1 is what useAuthStore returns
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.needsPin()).toBe(false);
  });

  it("needsPin returns false when content rating does not exceed max", async () => {
    const { useQuery } = await import("@tanstack/react-query");
    vi.mocked(useQuery).mockReturnValue({
      data: { enabled: true, hasPin: true, maxRating: "R" },
    } as ReturnType<typeof useQuery>);
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("PG")); // PG <= R
    expect(result.current.needsPin()).toBe(false);
  });
});
