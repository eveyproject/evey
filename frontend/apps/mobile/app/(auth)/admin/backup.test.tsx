import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
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
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent, ListHeaderComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "backups-list" },
      ListHeaderComponent ?? null,
      data.length === 0
        ? ListEmptyComponent ?? null
        : data.map((item: any) =>
            React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
          ),
    ),
  Alert: { alert: (_title: string, _msg: string, buttons: any[]) => buttons?.[1]?.onPress?.() },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- expo-router ------------------------------------------------------------------------
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) => React.createElement("div", { "data-testid": "stack-screen", title: options?.title }),
  },
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
      textMuted: "#888", accent: "#f00", error: "#e00", border: "#333", onAccent: "#fff", warning: "#fa0", success: "#0a0",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/store -------------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (selector: (s: any) => unknown) => selector({ accessToken: "tok" }),
  useDemoStore: (sel: (s: any) => unknown) => sel({ demoMode: false, demoLocked: [] }), demoLocks: (s: any, k: string) => s.demoMode || s.demoLocked.includes(k),
}));

// --- backup file ops (native expo-file-system I/O, irrelevant to this test) -------------
vi.mock("../../../src/components/admin/backup/backupFileOps", () => ({
  downloadBackupFile: vi.fn(),
  restoreExistingBackup: vi.fn(),
  restoreFromFile: vi.fn(),
}));

// expo-document-picker pulls in expo-modules-core (native) at import; the
// screen imports it for the restore-from-file picker.
vi.mock("expo-document-picker", () => ({ getDocumentAsync: vi.fn() }));

// --- @evey/api -----------------------------------------------------------------------------
// Spreads the real module: the screen renders backup sizes with formatBytes
// from @evey/api, which a mock listing only eveyFetch leaves undefined.
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
}));

// --- @tanstack/react-query -----------------------------------------------------------------
// useQuery is shared by useBackups() (BACKUPS_KEY) and ScheduleSection (SCHEDULE_KEY);
// the mock below dispatches on queryKey so each test can drive the backups list state
// independently from the (unrelated) schedule panel.
let backupsQueryState: { data: unknown; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("@tanstack/react-query", () => ({
  useQuery: (opts: { queryKey: unknown[] }) => {
    if (opts.queryKey[1] === "backups") return backupsQueryState;
    return { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
  },
  useMutation: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false, variables: undefined }),
  useQueryClient: () => ({ invalidateQueries: vi.fn(), setQueryData: vi.fn() }),
}));

// -------------------------------------------------------------------------------------
import AdminBackupScreen from "./backup";

beforeEach(() => {
  vi.clearAllMocks();
  backupsQueryState = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("AdminBackupScreen - load failure vs empty", () => {
  it("shows neither the empty message nor a retry while loading", () => {
    backupsQueryState = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(<AdminBackupScreen />);

    expect(screen.queryByText("admin.backup.noBackups")).toBeNull();
    expect(screen.queryByText("common.retry")).toBeNull();
  });

  it("shows LoadFailed with a retry, not the empty-backups message, when the fetch errors", () => {
    backupsQueryState = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    render(<AdminBackupScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.getByText("common.retry")).toBeTruthy();
    expect(screen.queryByText("admin.backup.noBackups")).toBeNull();
  });

  it("shows the empty-backups message, not LoadFailed, when the fetch succeeds with zero backups", () => {
    backupsQueryState = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
    render(<AdminBackupScreen />);

    expect(screen.getByText("admin.backup.noBackups")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
    expect(screen.queryByText("common.retry")).toBeNull();
  });
});
