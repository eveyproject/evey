import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const { replaceMock, lock, setState } = vi.hoisted(() => ({
  replaceMock: vi.fn(),
  lock: { state: { closed: false, nextOpen: undefined as string | undefined }, unlock: vi.fn() },
  setState: vi.fn(),
}));

vi.mock("../tv/focus", () => ({
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
}));
vi.mock("./ui", () => ({
  Pressable: ({ children, onPress }: any) => <button type="button" onClick={onPress}>{children}</button>,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: unknown) => (typeof f === "string" ? f : k), i18n: { language: "en" } }) }));
vi.mock("../app/router", () => ({ replace: (...a: unknown[]) => replaceMock(...a) }));
vi.mock("../music/queue", () => ({ useMusicQueueStore: { setState } }));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: any) => sel({ profileId: "kid" }) }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("@evey/client", () => ({
  useProfileLock: () => ({ ...lock.state, unlock: lock.unlock, recheck: vi.fn() }),
  formatOpening: () => "today 18:00",
}));
vi.mock("./ParentalPINModal", () => ({
  ParentalPINModal: ({ submitPin, onVerified }: any) => (
    <button
      type="button"
      onClick={async () => {
        const err = await submitPin("1234");
        if (err === null) onVerified();
        else document.body.setAttribute("data-pin-error", err);
      }}
    >
      keypad
    </button>
  ),
}));

import { ProfileLockScreen } from "./ProfileLockScreen";

beforeEach(() => {
  lock.state = { closed: false, nextOpen: undefined };
  lock.unlock.mockReset();
  replaceMock.mockReset();
  setState.mockReset();
  document.body.removeAttribute("data-pin-error");
});

// The TV web lock: stops the music, sends the keypad's PIN to the
// opening-hours unlock, and offers the profile picker.
describe("TV web ProfileLockScreen", () => {
  it("renders nothing while open", () => {
    const { container } = render(<ProfileLockScreen />);
    expect(container.innerHTML).toBe("");
  });

  it("locks, stops the music, unlocks through the keypad, or switches profile", async () => {
    lock.state = { closed: true, nextOpen: "2026-10-05T16:00:00Z" };
    const { rerender } = render(<ProfileLockScreen />);
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(setState).toHaveBeenCalledWith({ isPlaying: false });

    lock.unlock.mockResolvedValueOnce("wrong_pin");
    fireEvent.click(screen.getByText("Parent PIN"));
    fireEvent.click(screen.getByText("keypad"));
    await waitFor(() => expect(document.body.getAttribute("data-pin-error")).toBe("Wrong parent PIN"));

    rerender(<ProfileLockScreen />);
    lock.state = { closed: true, nextOpen: undefined };
    const fresh = render(<ProfileLockScreen />);
    fireEvent.click(fresh.getAllByText("Switch profile")[0]);
    expect(replaceMock).toHaveBeenCalledWith({ name: "profiles", pick: true });
  });
});
