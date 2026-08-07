import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("button", {
      type: "button",
      role: "switch",
      "aria-checked": !!value,
      onClick: () => onValueChange?.(!value),
    }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent, ListHeaderComponent, ListFooterComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "users-list" },
      ListHeaderComponent ?? null,
      data.length === 0 ? ListEmptyComponent ?? null : data.map((item: any) =>
        React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
      ),
      ListFooterComponent ?? null,
    ),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Alert: { alert: (_title: string, _msg: string, buttons: any[]) => buttons?.[1]?.onPress?.() },
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Share: { share: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- expo-router ------------------------------------------------------------------------
vi.mock("expo-router", () => ({
  Stack: {
    // headerRight is invoked so the "+" button that opens the create-user
    // sheet is reachable from the test, matching how the native header
    // renders it in the real app.
    Screen: ({ options }: any) =>
      React.createElement("div", { "data-testid": "stack-screen" }, options?.headerRight?.()),
  },
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", error: "#e00", border: "#333", onAccent: "#fff",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- useServerUrl -----------------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "https://evey.example.com",
}));

// --- BitrateModal (unrelated to this test, stubbed out) --------------------------------
vi.mock("./users/BitrateModal", () => ({
  BitrateModal: () => null,
}));

// --- @evey/store -------------------------------------------------------------------------
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { success: (...a: unknown[]) => toastSuccess(...a), error: (...a: unknown[]) => toastError(...a) },
  useAuthStore: (selector: (s: any) => unknown) => selector({ impersonate: vi.fn() }),
}));

// --- @tanstack/react-query -----------------------------------------------------------------
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  useQuery: () => ({ data: undefined, isLoading: false }),
}));

// --- @evey/api -----------------------------------------------------------------------------
// createUser.mutateAsync mimics react-query: it runs the mutation's onSuccess
// callback (which is where the buggy unconditional toast lives) before
// resolving, exactly as the real hook would once the create POST completes.
let libraryAccessShouldFail = false;
const eveyFetchMock = vi.fn(async (opts: { url: string; method: string }) => {
  if (opts.url.includes("library-access") && opts.method === "PUT") {
    if (libraryAccessShouldFail) throw new Error("library access failed");
    return undefined;
  }
  return undefined;
});

let mockUsers: any[] = [];
const deleteMutate = vi.fn();
const disableMutate = vi.fn();
const impersonateMutate = vi.fn();

vi.mock("@evey/api", () => ({
  useUserServiceListUsers: () => ({ data: { users: mockUsers }, isLoading: false, refetch: vi.fn() }),
  useUserServiceCreateUser: (opts: any) => ({
    mutateAsync: async (arg: any) => {
      const data = { id: "new-user-1" };
      await opts.mutation.onSuccess?.(data, arg, undefined);
      return data;
    },
    isPending: false,
  }),
  useUserServiceDeleteUser: () => ({ mutate: deleteMutate, isPending: false }),
  useUserServiceDisableUser: () => ({ mutate: disableMutate, isPending: false }),
  useUserServiceImpersonateUser: () => ({ mutate: impersonateMutate, isPending: false }),
  getUserServiceListUsersQueryKey: () => ["users"],
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib1", name: "Movies" }] } }),
  useUserServiceListInviteLinks: () => ({ data: { inviteLinks: [] }, isLoading: false }),
  useUserServiceCreateInviteLink: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useUserServiceRevokeInviteLink: () => ({ mutate: vi.fn(), isPending: false }),
  getUserServiceListInviteLinksQueryKey: () => ["invites"],
  eveyFetch: (...a: Parameters<typeof eveyFetchMock>) => eveyFetchMock(...a),
}));

// -------------------------------------------------------------------------------------
import AdminUsersScreen from "./users";

beforeEach(() => {
  vi.clearAllMocks();
  libraryAccessShouldFail = false;
  mockUsers = [];
});

async function openCreateFormAndSubmit() {
  render(<AdminUsersScreen />);

  // Open the create-user sheet via the header "+" button.
  fireEvent.click(screen.getByTestId("icon-add").closest("button")!);

  fireEvent.change(screen.getByLabelText("admin.users.username *"), { target: { value: "alice" } });
  fireEvent.change(screen.getByLabelText("admin.users.password *"), { target: { value: "s3cret-pw" } });

  // Select the one available library so the create flow takes the
  // library-access assignment branch.
  fireEvent.click(screen.getByText("Movies"));

  fireEvent.click(screen.getByText("common.create"));

  await waitFor(() => expect(eveyFetchMock).toHaveBeenCalled());
}

describe("AdminUsersScreen - create user honesty", () => {
  it("reports success when the user AND library access are both applied", async () => {
    libraryAccessShouldFail = false;
    await openCreateFormAndSubmit();

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(toastError).not.toHaveBeenCalled();
  });

  it("does not silently report success when library access assignment fails", async () => {
    libraryAccessShouldFail = true;
    await openCreateFormAndSubmit();

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    // The user WAS created but access was not applied - a plain
    // "user created" success toast here would be misleading.
    expect(toastSuccess).not.toHaveBeenCalled();
  });
});

const alice = { id: "u1", username: "alice", email: "alice@example.com", isAdmin: false, enabled: true };
const bob = { id: "u2", username: "bob", email: "bob@example.com", isAdmin: true, enabled: true };
const carol = { id: "u3", username: "carol", email: "carol@example.com", isAdmin: false, enabled: false };

function openSheetFor(username: string) {
  fireEvent.click(screen.getByText(username).closest("button")!);
}

describe("AdminUsersScreen - user row action sheet", () => {
  it("renders each user as a single row button without inline action icons", () => {
    mockUsers = [alice];
    render(<AdminUsersScreen />);

    expect(screen.getByText("alice")).toBeTruthy();
    // The old layout packed six icon buttons in the row; they must be gone
    // until the sheet is opened.
    expect(screen.queryByTestId("icon-trash-outline")).toBeNull();
    expect(screen.queryByTestId("icon-pencil-outline")).toBeNull();
  });

  it("opens a labelled action sheet on row tap and routes delete through it", () => {
    mockUsers = [alice];
    render(<AdminUsersScreen />);

    openSheetFor("alice");
    expect(screen.getByText("Edit")).toBeTruthy();
    expect(screen.getByText("Library Access")).toBeTruthy();
    expect(screen.getByText("Connect as")).toBeTruthy();

    // The Alert mock auto-confirms, so tapping Delete must reach the mutation.
    fireEvent.click(screen.getByText("Delete"));
    expect(deleteMutate).toHaveBeenCalledWith({ id: "u1" });
  });

  it("adapts the sheet to the user's state: admins have no library access, disabled users re-enable", () => {
    mockUsers = [bob, carol];
    render(<AdminUsersScreen />);

    openSheetFor("bob");
    expect(screen.queryByText("Library Access")).toBeNull();

    // Close bob's sheet, open carol's (disabled account).
    fireEvent.click(screen.getAllByText("Edit")[0].closest("button")!);
    openSheetFor("carol");
    expect(screen.queryByText("Connect as")).toBeNull();
    expect(screen.getByText("Enable")).toBeTruthy();
    fireEvent.click(screen.getByText("Enable"));
    expect(disableMutate).toHaveBeenCalledWith({ id: "u3", params: { disabled: false } });
  });

  it("filters the list from the search box once 8+ users exist", () => {
    mockUsers = Array.from({ length: 8 }, (_, i) => ({
      id: `u${i}`, username: `user${i}`, email: `user${i}@example.com`, isAdmin: false, enabled: true,
    }));
    render(<AdminUsersScreen />);

    expect(screen.getByText("user7")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search users"), { target: { value: "user7" } });
    expect(screen.getByText("user7")).toBeTruthy();
    expect(screen.queryByText("user1")).toBeNull();
  });

  it("hides the search box on small installs", () => {
    mockUsers = [alice, bob];
    render(<AdminUsersScreen />);
    expect(screen.queryByLabelText("Search users")).toBeNull();
  });
});
