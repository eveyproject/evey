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
// Plugins that mount the user-settings slot, per test.
const pluginList = vi.hoisted(() => ({ current: [] as unknown[] }));

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
  eveyFetch: vi.fn((req: { url?: string }) =>
    req?.url === "/api/v1/plugins"
      ? Promise.resolve({ plugins: pluginList.current })
      : Promise.resolve({}),
  ),
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

vi.mock("@evey/store", async (importActual) => ({
  // The pure preference helpers (autoplayNextEpisode, ...) come from the real
  // module; only the stores are stubbed.
  ...(await importActual<typeof import("@evey/store")>()),
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

vi.mock("@/features/user/QuickConnectSettings", () => ({
  QuickConnectSettings: () => React.createElement("div", { "data-testid": "quickconnect-settings" }, "QuickConnect Settings"),
}));

vi.mock("@/features/user/AutoplayNextSettings", () => ({
  AutoplayNextSettings: () => React.createElement("div", { "data-testid": "autoplay-next-settings" }, "Autoplay Next"),
}));

// The real slot renders a sandboxed iframe; here it only has to say WHICH
// plugin the page asked it for.
vi.mock("@/features/plugins/PluginSlot", () => ({
  PluginSlot: ({ name, pluginId }: { name: string; pluginId?: string }) =>
    React.createElement("div", { "data-testid": "plugin-slot", "data-slot": name, "data-plugin": pluginId ?? "" }),
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
    pluginList.current = [];
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

  it("credits TMDB and links the privacy policy", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    const credit = screen.getByText("This product uses the TMDB API but is not endorsed or certified by TMDB.");
    expect(credit.closest("a")).toHaveAttribute("href", "https://www.themoviedb.org/");
    expect(document.querySelector('a[href$="/privacy/"]')).not.toBeNull();
  });

  it("renders all tab buttons", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Appearance/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Notifications/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Parental/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Quick Connect/i })).toBeInTheDocument();
    // The push-devices tab is gone: nothing ever registers a native device.
    expect(screen.queryByRole("button", { name: /Devices/i })).not.toBeInTheDocument();
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

  // AUTH-3: a stolen token alone must not change the account's credentials.
  it("sends the current password with a credential change, and refuses without it", async () => {
    const mutate = vi.fn();
    mockUpdateUser.mockReturnValue({ mutate, isPending: false });
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: "Edit Profile" }));
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "new@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutate).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/Current password/i), { target: { value: "oldpassword" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutate).toHaveBeenCalledWith({
      id: "u-1",
      data: { email: "new@example.com", currentPassword: "oldpassword" },
    });
  });
});

// The menu used to be a horizontal tab strip: the six translated labels never
// fit its container, so reaching the last entries meant dragging sideways with
// no visible scrollbar. These lock the shape that replaced it.
describe("SettingsPage menu", () => {
  it("never asks the reader to scroll sideways: no horizontal scroller, no nowrap labels", async () => {
    const SettingsPage = await getSettingsPage();
    const { container } = render(React.createElement(SettingsPage), { wrapper: createWrapper() });

    const nav = container.querySelector("nav");
    expect(nav).not.toBeNull();
    expect(nav?.className).not.toContain("overflow-x");
    const firstTab = screen.getByTestId("profile-settings-tab-appearance");
    expect(firstTab.className).not.toContain("whitespace-nowrap");
    // The label is free to wrap onto a second line instead of widening the row.
    expect(firstTab.querySelector("span.min-w-0")?.textContent).toBe("Appearance");
  });

  it("keeps every section reachable, playback included", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });

    for (const key of ["appearance", "playback", "profiles", "notifications", "parental", "quick-connect"]) {
      expect(screen.getByTestId(`profile-settings-tab-${key}`)).toBeInTheDocument();
    }
    fireEvent.click(screen.getByTestId("profile-settings-tab-playback"));
    expect(screen.getByTestId("autoplay-next-settings")).toBeInTheDocument();
  });
});

describe("SettingsPage plugins menu", () => {
  const weather = {
    id: "weather", name: "Weather", enabled: true, webRoot: "dist",
    frontend: [{ slot: "settings.user.appearance", src: "dist/settings.js" }],
  };
  const notes = {
    id: "notes", name: "Notes", enabled: true, webRoot: "dist",
    frontend: [{ slot: "settings.user.appearance", src: "dist/settings.js" }],
  };

  it("hides the Plugins entry when nothing configurable is installed", async () => {
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByTestId("profile-settings-tab-appearance")).toBeInTheDocument());
    expect(screen.queryByTestId("profile-settings-tab-plugins")).not.toBeInTheDocument();
  });

  it("lists one sub-entry per plugin and opens the first one", async () => {
    pluginList.current = [weather, notes];
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });

    const entry = await screen.findByTestId("profile-settings-tab-plugins");
    fireEvent.click(entry);
    expect(screen.getByTestId("profile-settings-plugin-weather")).toHaveTextContent("Weather");
    expect(screen.getByTestId("profile-settings-plugin-notes")).toHaveTextContent("Notes");
    expect(screen.getByTestId("plugin-slot")).toHaveAttribute("data-plugin", "weather");
  });

  it("shows only the plugin picked in the sub-menu", async () => {
    pluginList.current = [weather, notes];
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });

    fireEvent.click(await screen.findByTestId("profile-settings-tab-plugins"));
    fireEvent.click(screen.getByTestId("profile-settings-plugin-notes"));
    const slots = screen.getAllByTestId("plugin-slot");
    expect(slots).toHaveLength(1);
    expect(slots[0]).toHaveAttribute("data-plugin", "notes");
    expect(slots[0]).toHaveAttribute("data-slot", "settings.user.appearance");
  });

  it("keeps the sub-menu out of the way until the Plugins entry is open", async () => {
    pluginList.current = [weather];
    const SettingsPage = await getSettingsPage();
    render(React.createElement(SettingsPage), { wrapper: createWrapper() });

    await screen.findByTestId("profile-settings-tab-plugins");
    expect(screen.queryByTestId("profile-settings-plugin-weather")).not.toBeInTheDocument();
  });
});
