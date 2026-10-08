import { clearUnlocked } from "./useParentalGate";
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
    clearUnlocked();
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
    clearUnlocked();
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

describe("settingsLoading (gate readiness)", () => {
  beforeEach(() => {
    clearUnlocked();
    vi.clearAllMocks();
  });

  // The player page used to evaluate needsPin() while the parental settings
  // were still in flight: needsPin() returned false, the player mounted and a
  // transcode session started BEFORE the PIN dialog appeared. The hook must
  // say "not ready yet" so callers can hold the spinner instead.
  it("reports loading while the settings query is in flight for a profile", async () => {
    // @ts-expect-error test-only partial shape
    mockUseParental.mockReturnValue({ data: undefined, isLoading: true });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.settingsLoading).toBe(true);
  });

  it("is not loading once the settings landed", async () => {
    // @ts-expect-error test-only partial shape
    mockUseParental.mockReturnValue({ data: { enabled: true, hasPin: true, maxAge: 10 }, isLoading: false });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.settingsLoading).toBe(false);
  });

  it("is not loading when there is no active profile (query disabled)", async () => {
    const { useAuthStore } = await import("@evey/store");
    // @ts-expect-error test-only mock override
    useAuthStore.mockImplementation((selector: (s: { profileId: string | null }) => unknown) =>
      selector({ profileId: null }));
    // Disabled TanStack v5 queries stay status=pending but never fetch;
    // isLoading (pending AND fetching) is false - the hook must not hang on it.
    // @ts-expect-error test-only partial shape
    mockUseParental.mockReturnValue({ data: undefined, isLoading: false });
    const { renderHook } = await import("@testing-library/react");
    const { useParentalGate } = await import("./useParentalGate");
    const { result } = renderHook(() => useParentalGate("R"));
    expect(result.current.settingsLoading).toBe(false);
  });
});

// The table is the server's (rendered by its test), not a 12-country subset:
// a certificate the old copy did not know played without the PIN prompt.
describe("isAgeExceeded reads the server's full table", () => {
  it("gates on the countries the hand-kept copy lacked", async () => {
    const { isAgeExceeded } = await import("./useParentalGate");
    expect(isAgeExceeded("CA", "14A", 12)).toBe(true);
    expect(isAgeExceeded("PT", "M/12", 10)).toBe(true);
    expect(isAgeExceeded("KR", "15", 15)).toBe(false);
    expect(isAgeExceeded("mx", " b-15 ", 13)).toBe(true);
  });

  it("still defers to the server on what it cannot resolve", async () => {
    const { isAgeExceeded } = await import("./useParentalGate");
    // "Not rated" is -1 in the table: unknown, never an age to compare.
    expect(isAgeExceeded("US", "NR", 0)).toBe(false);
    expect(isAgeExceeded("ZZ", "18", 0)).toBe(false);
    expect(isAgeExceeded("FR", undefined, 0)).toBe(false);
    // The US fallback the server also applies.
    expect(isAgeExceeded("FR", "PG-13", 12)).toBe(true);
  });
});
