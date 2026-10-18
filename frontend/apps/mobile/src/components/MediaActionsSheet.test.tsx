import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled, testID, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({}) }));
const markWatched = vi.hoisted(() => vi.fn());
const markUnwatched = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({
  displayTitle: (i: { title?: string }) => i.title ?? "",
  usePlaybackServiceMarkAsWatched: () => ({ mutate: markWatched }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: markUnwatched }),
}));
vi.mock("@evey/client", () => ({ invalidateWatchedState: vi.fn() }));
const auth = vi.hoisted(() => ({ profileId: "prf-1", isAdmin: false, demoMode: false }));
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel: (s: typeof auth) => unknown) => sel(auth),
  useDemoStore: (sel: (s: typeof auth) => unknown) => sel(auth),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 15 } }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { text: "#fff" }, radii: { md: 8 } }) }));
const toggleFav = vi.hoisted(() => vi.fn());
vi.mock("../hooks/useFavorite", () => ({
  useIsFavorite: () => ({ data: true }),
  useToggleFavorite: () => ({ mutate: toggleFav }),
}));
vi.mock("./BottomSheet", () => ({
  BottomSheet: ({ visible, title, children }: any) => (visible ? React.createElement("div", { "aria-label": title }, children) : null),
}));
vi.mock("./AddToPlaylistSheet", () => ({ AddToPlaylistSheet: () => null }));
vi.mock("./library/BulkAddSheets", () => ({ BulkAddToCollectionSheet: () => null }));

import { MediaActionsSheet } from "./MediaActionsSheet";

const item = { id: "m1", title: "Dune", type: "MEDIA_ITEM_TYPE_MOVIE" } as any;

beforeEach(() => {
  vi.clearAllMocks();
  auth.isAdmin = false;
  auth.demoMode = false;
});

// The long-press menu of a home card carries the web card menu's entries,
// behind the same admin gates.
describe("MediaActionsSheet", () => {
  it("shows the viewer entries only, and nothing without an item", () => {
    const { rerender } = render(<MediaActionsSheet item={null} onClose={() => {}} />);
    expect(screen.queryByTestId("media-actions-mark-watched")).toBeNull();
    rerender(<MediaActionsSheet item={item} onClose={() => {}} />);
    expect(screen.getByLabelText("Dune")).toBeTruthy();
    for (const k of ["add-to-playlist", "remove-from-favorites", "mark-watched", "mark-unwatched"]) {
      expect(screen.getByTestId(`media-actions-${k}`)).toBeTruthy();
    }
    expect(screen.queryByTestId("media-actions-edit-metadata")).toBeNull();
    expect(screen.queryByTestId("media-actions-add-to-collection")).toBeNull();
  });

  it("gives an admin the editor and the collections too; edit opens the title's editor", () => {
    auth.isAdmin = true;
    const onClose = vi.fn();
    render(<MediaActionsSheet item={item} onClose={onClose} />);
    expect(screen.getByTestId("media-actions-add-to-collection")).toBeTruthy();
    fireEvent.click(screen.getByTestId("media-actions-edit-metadata"));
    expect(onClose).toHaveBeenCalled();
    expect(router.push).toHaveBeenCalledWith("/media/m1?edit=1");
  });

  it("marks watched or unwatched for the active profile, and flips the favourite from its shown state", () => {
    render(<MediaActionsSheet item={item} onClose={() => {}} />);
    fireEvent.click(screen.getByTestId("media-actions-mark-watched"));
    expect(markWatched).toHaveBeenCalledWith({ mediaItemId: "m1", params: { profileId: "prf-1" } });
    fireEvent.click(screen.getByTestId("media-actions-mark-unwatched"));
    expect(markUnwatched).toHaveBeenCalledWith({ mediaItemId: "m1", params: { profileId: "prf-1" } });
    fireEvent.click(screen.getByTestId("media-actions-remove-from-favorites"));
    expect(toggleFav).toHaveBeenCalledWith(true);
  });

  // On the demo every entry but the editor writes the shared account's
  // content, which the server refuses: greyed, as on the web card menu, and
  // a press reaches no mutation.
  it("greys every entry but the editor on the demo", () => {
    auth.isAdmin = true;
    auth.demoMode = true;
    const onClose = vi.fn();
    render(<MediaActionsSheet item={item} onClose={onClose} />);
    for (const k of ["add-to-playlist", "add-to-collection", "remove-from-favorites", "mark-watched", "mark-unwatched"]) {
      const b = screen.getByTestId(`media-actions-${k}`) as HTMLButtonElement;
      expect(b.disabled).toBe(true);
      fireEvent.click(b);
    }
    expect(markWatched).not.toHaveBeenCalled();
    expect(markUnwatched).not.toHaveBeenCalled();
    expect(toggleFav).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    const edit = screen.getByTestId("media-actions-edit-metadata") as HTMLButtonElement;
    expect(edit.disabled).toBeFalsy();
    fireEvent.click(edit);
    expect(router.push).toHaveBeenCalledWith("/media/m1?edit=1");
  });
});
