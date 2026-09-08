import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockListOnline = vi.fn();
const mockSendCastCommand = vi.fn();
const mockListAllProfiles = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePresenceServiceListOnline: () => mockListOnline(),
  useAdminSessionServiceSendCastReceiverCommand: () => ({ mutate: mockSendCastCommand }),
  useUserServiceListAllProfiles: () => mockListAllProfiles(),
}));

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

async function OnlineNow() {
  const mod = await import("./DashboardOnline");
  return mod.OnlineNow;
}

const CAST_ROW = {
  userId: "usr-1",
  username: "Someone",
  profileId: "prf-1",
  userAgent: "EveyCast (Chromecast)",
  isCastReceiver: true,
  idleSeconds: 12,
};

const BROWSER_ROW = {
  userId: "usr-2",
  username: "Someone Else",
  profileId: "prf-2",
  userAgent: "Mozilla/5.0",
  isCastReceiver: false,
  idleSeconds: 3,
};

// Every describe needs the profile list mocked: the card resolves the profile
// NAME from it, and an unmocked hook would hand back undefined.
const PROFILES = [
  { id: "prf-1", name: "Household", isDefault: true },
  { id: "prf-2", name: "Camille", isDefault: false },
];

describe("OnlineNow: stopping a receiver that outlived its session", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListOnline.mockReturnValue({ data: { users: [CAST_ROW], total: 1 }, isError: false });
    mockListAllProfiles.mockReturnValue({ data: { profiles: PROFILES } });
  });

  // The whole point of the control: a television that never really switches off
  // keeps its receiver page loaded, so it goes on beating long after its play
  // session ended - and Now Playing, which is where the remote control lives,
  // no longer lists it.
  it("offers a stop button on a beaconing cast receiver", async () => {
    const C = await OnlineNow();
    render(React.createElement(C));
    expect(screen.getByTestId("stop-cast-receiver")).toBeInTheDocument();
  });

  it("offers nothing on an ordinary browser - there is no television to release", async () => {
    mockListOnline.mockReturnValue({ data: { users: [BROWSER_ROW], total: 1 }, isError: false });
    const C = await OnlineNow();
    render(React.createElement(C));
    expect(screen.queryByTestId("stop-cast-receiver")).not.toBeInTheDocument();
  });

  // This ends someone else's playback on a screen the operator cannot see.
  it("arms on the first click and sends nothing yet", async () => {
    const C = await OnlineNow();
    render(React.createElement(C));
    fireEvent.click(screen.getByTestId("stop-cast-receiver"));
    expect(mockSendCastCommand).not.toHaveBeenCalled();
  });

  it("sends the stop on the second click, keyed by profile", async () => {
    const C = await OnlineNow();
    render(React.createElement(C));
    const btn = screen.getByTestId("stop-cast-receiver");
    fireEvent.click(btn);
    fireEvent.click(btn);
    expect(mockSendCastCommand).toHaveBeenCalledWith({ profileId: "prf-1", data: { command: "stop" } });
  });

  // A row with no profile has nothing the server could route the command to;
  // rendering the button would offer an action that silently does nothing.
  it("hides the button when the row carries no profile", async () => {
    mockListOnline.mockReturnValue({
      data: { users: [{ ...CAST_ROW, profileId: "" }], total: 1 },
      isError: false,
    });
    const C = await OnlineNow();
    render(React.createElement(C));
    expect(screen.queryByTestId("stop-cast-receiver")).not.toBeInTheDocument();
  });
});

// An admin reading this card needs to know WHO is watching. The account name
// is the household; the profile is the person.
describe("OnlineNow: which profile is watching", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListAllProfiles.mockReturnValue({ data: { profiles: PROFILES } });
  });

  it("names the profile next to the device", async () => {
    mockListOnline.mockReturnValue({ data: { users: [BROWSER_ROW], total: 1 }, isError: false });
    const Comp = await OnlineNow();
    render(React.createElement(Comp));
    expect(screen.getByText(/Camille/)).toBeInTheDocument();
  });

  it("says Default for the household profile instead of repeating the account name", async () => {
    mockListOnline.mockReturnValue({ data: { users: [CAST_ROW], total: 1 }, isError: false });
    const Comp = await OnlineNow();
    render(React.createElement(Comp));
    expect(screen.getByText(/Default/)).toBeInTheDocument();
    expect(screen.queryByText(/Household/)).not.toBeInTheDocument();
  });

  it("shows no profile rather than a raw id when the profile is gone", async () => {
    mockListOnline.mockReturnValue({
      data: { users: [{ ...BROWSER_ROW, profileId: "prf-deleted" }], total: 1 },
      isError: false,
    });
    const Comp = await OnlineNow();
    render(React.createElement(Comp));
    expect(screen.queryByText(/prf-deleted/)).not.toBeInTheDocument();
    expect(screen.getByText(/Someone Else/)).toBeInTheDocument();
  });

  it("survives a profile list that has not arrived yet", async () => {
    mockListAllProfiles.mockReturnValue({ data: undefined });
    mockListOnline.mockReturnValue({ data: { users: [BROWSER_ROW], total: 1 }, isError: false });
    const Comp = await OnlineNow();
    render(React.createElement(Comp));
    expect(screen.getByText(/Someone Else/)).toBeInTheDocument();
  });
});
