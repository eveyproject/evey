import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const { fetchMock, replaceMock } = vi.hoisted(() => ({ fetchMock: vi.fn(), replaceMock: vi.fn() }));

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ replace: (...a: unknown[]) => replaceMock(...a), goBack: vi.fn() }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...a: unknown[]) => fetchMock(...a),
}));

import { SyncplayScreen } from "./Syncplay";

beforeEach(() => { fetchMock.mockReset(); replaceMock.mockReset(); });

describe("joining a room", () => {
  // The room alphabet has no I, L, O or U: a keypad offering them lets the
  // viewer type a code that cannot exist.
  it("offers only the room alphabet", () => {
    render(<SyncplayScreen />);
    for (const k of ["I", "L", "O", "U"]) expect(screen.queryByLabelText(k)).toBeNull();
    for (const k of ["0", "9", "A", "Z"]) expect(screen.getByLabelText(k)).toBeTruthy();
  });

  it("joins on the sixth character and opens the player in the room", async () => {
    fetchMock.mockResolvedValue({ roomId: "room-1", mediaId: "m-1" });
    render(<SyncplayScreen />);
    for (const k of ["A", "B", "C", "1", "2", "3"]) fireEvent.click(screen.getByLabelText(k));
    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith({ name: "player", mediaId: "m-1", roomId: "room-1" }));
    expect(fetchMock).toHaveBeenCalledWith(expect.objectContaining({ data: { code: "ABC123" } }));
  });
});
