import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// Admin-only item removal from a collection (RemoveCollectionItem RPC, server
// this beta). The screen must only render the control for an admin, and never
// fire the mutation without the Alert.alert confirmation being accepted.

// --- React Native mock ----------------------------------------------------------------
const alertMock = vi.fn();
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TouchableOpacity: ({ children, onPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  Pressable: ({ children, onPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
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
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, i) : i },
          renderItem({ item, index: i }),
        ),
      ),
    );
  },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Dimensions: {
    get: () => ({ width: 400, height: 800 }),
    addEventListener: () => ({ remove: vi.fn() }),
  },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Alert: { alert: (...a: unknown[]) => alertMock(...a) },
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", null, children) : null),
  TextInput: ({ onChangeText, testID, ...p }: any) =>
    React.createElement("input", { ...p, "data-testid": testID, onChange: (e: any) => onChangeText?.(e.target.value) }),
  KeyboardAvoidingView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Platform: { OS: "android", select: (o: any) => o.android },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    // No real translation strings loaded here, so an interpolated key (called
    // without a fallback, like removeItemConfirm) can't render its {{title}}
    // placeholder - append the options instead so the interpolated value is
    // still observable in the rendered Alert message.
    t: (key: string, fallbackOrOpts?: unknown, maybeOpts?: Record<string, unknown>) => {
      const fallback = typeof fallbackOrOpts === "string" ? fallbackOrOpts : key;
      const opts = (typeof fallbackOrOpts === "object" ? fallbackOrOpts : maybeOpts) as Record<string, unknown> | undefined;
      if (!opts) return fallback;
      const interpolated = fallback.replace(/\{\{(\w+)\}\}/g, (_, k) => String(opts[k] ?? ""));
      return interpolated === fallback && interpolated === key ? `${key} ${JSON.stringify(opts)}` : interpolated;
    },
  }),
}));

// --- expo-router ---------------------------------------------------------------------
const { pushMock, backMock, COLLECTION_ID } = vi.hoisted(() => ({
  pushMock: vi.fn(),
  backMock: vi.fn(),
  COLLECTION_ID: "col-1",
}));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: COLLECTION_ID }),
  useRouter: () => ({ push: pushMock, back: backMock }),
  Stack: {
    // headerRight is invoked so the edit/delete header controls are reachable
    // from the test, matching how the native header renders them.
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
      danger: "#e00",
    },
    radii: { lg: 12, md: 8, sm: 4 },
  }),
}));

// --- hooks the screen pulls in for images -------------------------------------------
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
vi.mock("../components/Skeleton", () => ({ Skeleton: () => null }));

// --- @evey/store -----------------------------------------------------------------------
// Shape copied from packages/store/src/auth.ts: `isAdmin` is a plain boolean
// field on the store state, read through the selector like the screen does.
const { isAdminRef, toastSuccessMock, toastErrorMock } = vi.hoisted(() => ({
  isAdminRef: { current: true },
  toastSuccessMock: vi.fn(),
  toastErrorMock: vi.fn(),
}));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ isAdmin: isAdminRef.current }),
  toast: { success: toastSuccessMock, error: toastErrorMock },
}));

// --- src/api/collections ----------------------------------------------------------------
const { refetchMock, removeItemMutateAsyncMock, updateMutateAsyncMock, deleteMutateAsyncMock } = vi.hoisted(() => ({
  refetchMock: vi.fn(),
  removeItemMutateAsyncMock: vi.fn(),
  updateMutateAsyncMock: vi.fn(),
  deleteMutateAsyncMock: vi.fn(),
}));

const COLLECTION_DATA = {
  collection: { id: COLLECTION_ID, name: "Bond Saga", overview: "" },
  items: [
    { id: "mi-1", title: "Dr. No", type: "MEDIA_ITEM_TYPE_MOVIE" },
    { id: "mi-2", title: "GoldenEye", type: "MEDIA_ITEM_TYPE_MOVIE" },
  ],
  total: 2,
};

vi.mock("../api/collections", () => ({
  useCollection: () => ({ data: COLLECTION_DATA, isLoading: false, isError: false, refetch: refetchMock }),
  useUpdateCollection: () => ({ mutateAsync: updateMutateAsyncMock, isPending: false }),
  useDeleteCollection: () => ({ mutateAsync: deleteMutateAsyncMock, isPending: false }),
  useRemoveCollectionItem: () => ({ mutateAsync: removeItemMutateAsyncMock, isPending: false }),
}));

// -------------------------------------------------------------------------------------
import CollectionDetailScreen from "../../app/(auth)/collections/[id]";

function renderScreen() {
  return render(<CollectionDetailScreen />);
}

beforeEach(() => {
  vi.clearAllMocks();
  isAdminRef.current = true;
  removeItemMutateAsyncMock.mockResolvedValue(undefined);
});

describe("CollectionDetailScreen item removal", () => {
  it("shows the remove control for an admin and sends the RIGHT media item id after confirming", async () => {
    renderScreen();

    const removeBtn = screen.getByTestId("collection-remove-mi-2");
    fireEvent.click(removeBtn);

    expect(alertMock).toHaveBeenCalledTimes(1);
    const [, message, buttons] = alertMock.mock.calls[0];
    expect(message).toContain("GoldenEye");

    // Simulate the user tapping the destructive confirm button.
    const confirmButton = buttons.find((b: any) => b.style === "destructive");
    await confirmButton.onPress();

    expect(removeItemMutateAsyncMock).toHaveBeenCalledWith({ id: COLLECTION_ID, mediaItemId: "mi-2" });
    expect(removeItemMutateAsyncMock).not.toHaveBeenCalledWith({ id: COLLECTION_ID, mediaItemId: "mi-1" });
    await waitFor(() => expect(refetchMock).toHaveBeenCalled());
  });

  it("does not call the mutation when the confirmation is dismissed (cancel)", () => {
    renderScreen();

    fireEvent.click(screen.getByTestId("collection-remove-mi-1"));

    const [, , buttons] = alertMock.mock.calls[0];
    // Only the cancel entry exists in this path - no onPress to invoke.
    const cancelButton = buttons.find((b: any) => b.style === "cancel");
    expect(cancelButton.onPress).toBeUndefined();
    expect(removeItemMutateAsyncMock).not.toHaveBeenCalled();
  });

  it("hides the remove control entirely for a non-admin", () => {
    isAdminRef.current = false;
    renderScreen();

    expect(screen.queryByTestId("collection-remove-mi-1")).toBeNull();
    expect(screen.queryByTestId("collection-remove-mi-2")).toBeNull();
    // The screen must still be rendering the real items (not an empty state
    // masking the missing guard) so the "absent" assertions above are meaningful.
    expect(screen.getByText("Dr. No")).toBeTruthy();
    expect(screen.getByText("GoldenEye")).toBeTruthy();
  });
});

// UpdateCollection/DeleteCollection RPCs require RoleAdmin server-side
// (interceptors/rbac.go); the header edit/delete controls must follow the
// same admin-only convention as item removal above, not just error on tap.
// A rename must not cost a saga the synopsis TMDB gave it: the field is
// optional on the wire, and absent means keep.
describe("CollectionDetailScreen rename", () => {
  it("sends no overview when the description was left alone", async () => {
    renderScreen();
    fireEvent.click(screen.getByTestId("collection-edit-button"));
    fireEvent.change(screen.getByTestId("collection-edit-name"), { target: { value: "Bond - Saga" } });
    fireEvent.click(screen.getByTestId("collection-edit-save"));

    await waitFor(() => expect(updateMutateAsyncMock).toHaveBeenCalled());
    const sent = updateMutateAsyncMock.mock.calls[0][0];
    expect(sent.name).toBe("Bond - Saga");
    expect(sent).not.toHaveProperty("overview");
  });

  it("sends the overview once the description is edited", async () => {
    renderScreen();
    fireEvent.click(screen.getByTestId("collection-edit-button"));
    fireEvent.change(screen.getByTestId("collection-edit-description"), { target: { value: "Every 007 film" } });
    fireEvent.click(screen.getByTestId("collection-edit-save"));

    await waitFor(() => expect(updateMutateAsyncMock).toHaveBeenCalled());
    expect(updateMutateAsyncMock.mock.calls[0][0].overview).toBe("Every 007 film");
  });
});

describe("CollectionDetailScreen header edit/delete controls", () => {
  it("shows the edit and delete controls for an admin", () => {
    renderScreen();

    expect(screen.getByTestId("collection-edit-button")).toBeTruthy();
    expect(screen.getByTestId("collection-delete-button")).toBeTruthy();
  });

  it("hides the edit and delete controls entirely for a non-admin", () => {
    isAdminRef.current = false;
    renderScreen();

    expect(screen.queryByTestId("collection-edit-button")).toBeNull();
    expect(screen.queryByTestId("collection-delete-button")).toBeNull();
  });
});
