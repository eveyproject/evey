import { describe, it, expect, vi, beforeEach } from "vitest";
import type React from "react";
import { fireEvent, render, screen } from "@testing-library/react";

const mutate = vi.fn();
vi.mock("@evey/client", () => ({
  usePlaylistList: () => ({ data: [{ id: "pl-1", name: "Soirée", type: "manual" }], isLoading: false }),
  useAddToPlaylist: () => ({ mutate, isPending: false }),
}));
vi.mock("../tv/focus", () => ({
  useBackHandler: () => {},
  FocusRoot: ({ children, className }: { children?: React.ReactNode; className?: string }) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
  useArrivalFocus: () => {},
}));
const toastError = vi.fn();
vi.mock("@evey/store", () => ({ toast: { error: (...a: unknown[]) => toastError(...a), success: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { AddToPlaylistSheet } from "./AddToPlaylistSheet";

beforeEach(() => { mutate.mockClear(); toastError.mockClear(); });

// The shared mutation says nothing: the sheet words success and failure.
describe("AddToPlaylistSheet (tv-web)", () => {
  it("adds the title and words the outcome itself", () => {
    render(<AddToPlaylistSheet mediaItemId="mi-1" mediaType="movie" onClose={() => {}} />);
    fireEvent.click(screen.getByText("Soirée"));
    expect(mutate).toHaveBeenCalledWith({ playlistId: "pl-1", mediaItemId: "mi-1", mediaType: "movie" }, expect.any(Object));
    mutate.mock.calls[0][1].onError();
    expect(toastError).toHaveBeenCalledTimes(1);
  });
});
