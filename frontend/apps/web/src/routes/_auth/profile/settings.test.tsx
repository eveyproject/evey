import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetCurrentUser = vi.fn();
const mockListProfiles = vi.fn();
const mockUpdateUser = vi.fn();
const mockUpdateProfile = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();
const mockGetParentalSettings = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => mockGetCurrentUser(...args),
  useUserServiceListProfiles: (...args: unknown[]) => mockListProfiles(...args),
  useUserServiceUpdateUser: (...args: unknown[]) => mockUpdateUser(...args),
  useUserServiceUpdateProfile: (...args: unknown[]) => mockUpdateProfile(...args),
  useParentalServiceGetParentalSettings: (...args: unknown[]) => mockGetParentalSettings(...args),
  useMediaRequestServiceGetRequestPolicy: () => ({ data: { canRequest: false } }),
  parentalServiceUpdateParentalSettings: vi.fn(() => Promise.resolve({})),
  getParentalServiceGetParentalSettingsQueryKey: () => ["/api/v1/parental"],
  getUserServiceGetCurrentUserQueryKey: () => ["/api/v1/users/me"],
  getUserServiceListProfilesQueryKey: (id: string) => ["/api/v1/users", id, "profiles"],
  // Resolves to an empty payload: a bare vi.fn() hands react-query
  // `undefined`, which it rejects with a warning on every render.
  eveyFetch: vi.fn().mockResolvedValue({}),
  eveyFetchRaw: vi.fn(),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

// The profile the session is running as. Settable per test: which profile this
// screen reads and WRITES is the whole point of the cases below.
const authProfileId = vi.hoisted(() => ({ current: null as string | null }));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state = { accessToken: "fake-token", isAdmin: false, profileId: authProfileId.current };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ accessToken: "fake-token", isAdmin: false, profileId: authProfileId.current }) },
  ),
  useLocaleStore: (selector: (s: { locale: string; setLocale: () => void }) => unknown) =>
    selector({ locale: "en", setLocale: vi.fn() }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
  Link: ({ children, to, className }: { children: React.ReactNode; to: string; className?: string }) =>
    React.createElement("a", { href: to, className }, children),
}));

vi.mock("@/features/user/ThemeSettings", () => ({
  ThemeSettings: () => React.createElement("div", { "data-testid": "theme-settings" }, "Theme Settings"),
}));

vi.mock("@/features/user/NotificationSettings", () => ({
  NotificationSettings: () => React.createElement("div", { "data-testid": "notification-settings" }, "Notification Settings"),
}));

vi.mock("@/features/user/DeviceSettings", () => ({
  DeviceSettings: () => React.createElement("div", { "data-testid": "device-settings" }, "Device Settings"),
}));

vi.mock("@/features/user/QuickConnectSettings", () => ({
  QuickConnectSettings: () => React.createElement("div", { "data-testid": "quickconnect-settings" }, "QuickConnect Settings"),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getSettingsPage() {
  const mod = await import("./settings.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockUser = { id: "u-1", username: "alice", email: "alice@example.com" };
const mockProfile = {
  id: "prof-1",
  isDefault: true,
  maxRating: "PG",
  preferences: { max_bitrate_kbps: "4000", parental_enabled: "false" },
};

describe("SettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetCurrentUser.mockReturnValue({ data: mockUser });
    mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile] } });
    mockUpdateUser.mockReturnValue({ mutate: vi.fn(), isPending: false });
    mockUpdateProfile.mockReturnValue({ mutate: vi.fn(), isPending: false });
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
    mockGetParentalSettings.mockReturnValue({ data: { enabled: true, hasPin: false, maxAge: null } });
    authProfileId.current = null;
  });

  // The server enforces the PIN and the age cap of the ACTIVE profile
  // (checkParental reads claims.ProfileID). Resolving the DEFAULT profile here
  // showed a household member restrictions that were never applied to them,
  // and edited a profile they were not on. The write SUCCEEDS either way,
  // which is why only a test catches it.
  describe("targets the profile the session is running as", () => {
    const kid = { id: "prof-kid", isDefault: false, preferences: {} };

    it("reads the parental settings of the active profile, not the default one", async () => {
      authProfileId.current = "prof-kid";
      mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile, kid] } });
      const SettingsPage = await getSettingsPage();
      render(React.createElement(SettingsPage), { wrapper: createWrapper() });
      fireEvent.click(screen.getByRole("button", { name: /Parental/i }));

      await waitFor(() => expect(mockGetParentalSettings).toHaveBeenCalled());
      expect(mockGetParentalSettings.mock.calls.some((c) => c[0] === "prof-kid")).toBe(true);
      expect(mockGetParentalSettings.mock.calls.some((c) => c[0] === "prof-1")).toBe(false);
    });

    it("falls back to the default profile when the active id matches nothing", async () => {
      authProfileId.current = "prof-deleted";
      mockListProfiles.mockReturnValue({ data: { profiles: [mockProfile, kid] } });
      const SettingsPage = await getSettingsPage();
      render(React.createElement(SettingsPage), { wrapper: createWrapper() });
      fireEvent.click(screen.getByRole("button", { name: /Parental/i }));

      await waitFor(() => expect(mockGetParentalSettings).toHaveBeenCalled());
      expect(mockGetParentalSettings.mock.calls.some((c) => c[0] === "prof-1")).toBe(true);
    });
  });

  it("renders Settings heading", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Settings")).toBeInTheDocument();
  });

  it("renders all tab buttons", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Appearance/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Notifications/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Parental/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Devices/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Quick Connect/i })).toBeInTheDocument();
  });

  it("shows Appearance tab content by default", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("theme-settings")).toBeInTheDocument();
  });

  it("switching to Notifications tab shows notification settings", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Notifications/i }));
    await waitFor(() => expect(screen.getByTestId("notification-settings")).toBeInTheDocument());
  });

  it("switching to Parental tab shows the PIN management section", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Parental/i }));
    // The age-toggle moved to per-profile editing; the parental tab is now
    // strictly the PIN management form.
    await waitFor(() => expect(screen.getByLabelText(/New PIN/i)).toBeInTheDocument());
  });

  it("Parental tab shows PIN error when PINs do not match", async () => {
    const profile = { ...mockProfile, preferences: { ...mockProfile.preferences, parental_enabled: "true" } };
    mockListProfiles.mockReturnValue({ data: { profiles: [profile] } });
    // Pretend a PIN already exists so the Save PIN button isn't gated on the
    // first-time-set `currentPassword` field - the test only cares about the
    // client-side PIN/confirm mismatch path.
    mockGetParentalSettings.mockReturnValue({ data: { enabled: true, hasPin: true, maxAge: null } });
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Parental/i }));
    await waitFor(() => expect(screen.getByLabelText(/New PIN/i)).toBeInTheDocument());
    // Rotation now requires the current PIN to pass client-side validation
    // before reaching the new/confirm mismatch check.
    fireEvent.change(screen.getByLabelText(/Current PIN/i), { target: { value: "0000" } });
    fireEvent.change(screen.getByLabelText(/New PIN/i), { target: { value: "1234" } });
    fireEvent.change(screen.getByLabelText(/Confirm PIN/i), { target: { value: "5678" } });
    fireEvent.click(screen.getByRole("button", { name: /Update PIN/i }));
    await waitFor(() => expect(screen.getByText(/PINs do not match/i)).toBeInTheDocument());
  });

  it("renders ProfileCard with user info", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    expect(screen.getByText("alice")).toBeInTheDocument();
  });
});
