import React from "react";
import { render, screen, fireEvent, act, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mockNavigate }));

const lock = vi.hoisted(() => ({
  state: { closed: false, nextOpen: undefined as string | undefined },
  unlock: vi.fn(),
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useProfileLock: () => ({ ...lock.state, unlock: lock.unlock, recheck: vi.fn() }),
}));

import { ProfileLockOverlay } from "./ProfileLockOverlay";

function renderOverlay() {
  const qc = new QueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <ProfileLockOverlay />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  lock.state = { closed: false, nextOpen: undefined };
  lock.unlock.mockReset();
  mockNavigate.mockReset();
});

// Closed: the lock covers the app, stops what was playing, says when it opens,
// takes a parent's PIN or sends to the profile picker. Open: nothing.
describe("ProfileLockOverlay", () => {
  it("renders nothing while the profile is open", () => {
    const { container } = renderOverlay();
    expect(container.innerHTML).toBe("");
  });

  it("locks, pauses media, and unlocks with a parent PIN", async () => {
    const video = document.createElement("video");
    const pause = vi.spyOn(video, "pause").mockImplementation(() => {});
    document.body.appendChild(video);
    lock.state = { closed: true, nextOpen: new Date(Date.now() + 3600_000).toISOString() };
    renderOverlay();

    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText("This profile is locked right now")).toBeInTheDocument();
    expect(screen.getByText(/It opens again/)).toBeInTheDocument();
    expect(pause).toHaveBeenCalled();

    lock.unlock.mockResolvedValueOnce("wrong_pin");
    fireEvent.change(screen.getByLabelText("Parent PIN"), { target: { value: "9999" } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Unlock" }));
    });
    await waitFor(() => expect(screen.getByText("Wrong parent PIN")).toBeInTheDocument());
    expect(lock.unlock).toHaveBeenCalledWith("9999");

    fireEvent.click(screen.getByRole("button", { name: "Switch profile" }));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/profile-select" });
    video.remove();
  });

  it("says when no opening is planned", () => {
    lock.state = { closed: true, nextOpen: undefined };
    renderOverlay();
    expect(screen.getByText("No opening is planned in the coming days.")).toBeInTheDocument();
  });
});
