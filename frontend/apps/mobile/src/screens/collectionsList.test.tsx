import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// CreateCollection/DeleteCollection RPCs require RoleAdmin server-side
// (interceptors/rbac.go). The "+" header button (create) and the long-press
// delete gesture on a card must follow the same admin-only convention as
// item removal in collections/[id].tsx: hidden for a non-admin, not just
// erroring on tap. Long-press must keep the plain navigation tap working for
// everyone.

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TouchableOpacity: ({ children, onPress, onLongPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, onDoubleClick: onLongPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(React.Fragment, null, ListEmptyComponent) : null;
    }
    return React.createElement(
      "div",
      null,
      data.map((item: any, i: number) =>
        React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, i) : i }, renderItem({ item, index: i })),
      ),
    );
  },
  Dimensions: { get: () => ({ width: 400, height: 800 }), addEventListener: () => ({ remove: vi.fn() }) },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Alert: { alert: (_title: string, _msg: string, buttons: any[]) => buttons?.find((b: any) => b.style === "destructive")?.onPress?.() },
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", null, children) : null),
  TextInput: (p: any) => React.createElement("input", p),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  KeyboardAvoidingView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Platform: { OS: "android", select: (o: any) => o.android },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

// --- expo-router ---------------------------------------------------------------------
const { pushMock } = vi.hoisted(() => ({ pushMock: vi.fn() }));
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
  Stack: {
    // headerRight is invoked so the "+" create button is reachable from the
    // test, matching how the native header renders it.
    Screen: ({ options }: any) =>
      React.createElement("div", { "data-testid": "stack-screen" }, options?.headerRight?.()),
  },
}));

// --- ThemeProvider -------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      surfaceRaised: "#222",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
    },
    radii: { lg: 12, md: 8, sm: 4 },
  }),
}));

vi.mock("../components/Skeleton", () => ({ Skeleton: () => null }));

// --- @evey/store -----------------------------------------------------------------------
// Shape copied from packages/store/src/auth.ts: `isAdmin` is a plain boolean
// field on the store state, read through the selector like the screen does.
const { isAdminRef } = vi.hoisted(() => ({ isAdminRef: { current: true } }));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ isAdmin: isAdminRef.current }),
}));

// --- src/api/collections ----------------------------------------------------------------
const { createMutateAsyncMock, deleteMutateMock } = vi.hoisted(() => ({
  createMutateAsyncMock: vi.fn(),
  deleteMutateMock: vi.fn(),
}));

const COLLECTIONS = [
  { id: "col-1", name: "Bond Saga", itemCount: 2 },
  { id: "col-2", name: "Marvel", itemCount: 5 },
];

vi.mock("../api/collections", () => ({
  useCollections: () => ({ data: { collections: COLLECTIONS }, isLoading: false, refetch: vi.fn() }),
  useCreateCollection: () => ({ mutateAsync: createMutateAsyncMock, isPending: false }),
  useDeleteCollection: () => ({ mutate: deleteMutateMock, isPending: false }),
}));

// -------------------------------------------------------------------------------------
import CollectionsScreen from "../../app/(auth)/collections/index";

function renderScreen() {
  return render(<CollectionsScreen />);
}

beforeEach(() => {
  vi.clearAllMocks();
  isAdminRef.current = true;
});

describe("CollectionsScreen create button", () => {
  it("shows the create ('+') button for an admin", () => {
    renderScreen();

    expect(screen.getByTestId("icon-add-circle-outline")).toBeTruthy();
  });

  it("hides the create ('+') button entirely for a non-admin", () => {
    isAdminRef.current = false;
    renderScreen();

    expect(screen.queryByTestId("icon-add-circle-outline")).toBeNull();
    // The screen still renders the real collections underneath the guard.
    expect(screen.getByText("Bond Saga")).toBeTruthy();
  });
});

describe("CollectionsScreen long-press delete", () => {
  it("deletes on long-press for an admin", () => {
    renderScreen();

    fireEvent.doubleClick(screen.getByText("Bond Saga").closest("button")!);

    expect(deleteMutateMock).toHaveBeenCalledWith("col-1", expect.objectContaining({ onError: expect.any(Function) }));
  });

  it("does not delete on long-press for a non-admin, but keeps normal tap navigation", () => {
    isAdminRef.current = false;
    renderScreen();

    const card = screen.getByText("Bond Saga").closest("button")!;
    fireEvent.doubleClick(card);
    expect(deleteMutateMock).not.toHaveBeenCalled();

    fireEvent.click(card);
    expect(pushMock).toHaveBeenCalledWith("/collections/col-1");
  });
});
