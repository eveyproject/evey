import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

const { refreshToken, listProfiles, replaceMock, setTokens, setProfile, setRefreshToken } = vi.hoisted(() => ({
  refreshToken: vi.fn(),
  listProfiles: vi.fn(),
  replaceMock: vi.fn(),
  setTokens: vi.fn(),
  setProfile: vi.fn(),
  setRefreshToken: vi.fn(),
}));

vi.mock("../tv/focus", () => ({
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: unknown) => (typeof f === "string" ? f : k) }) }));
vi.mock("../app/router", () => ({ replace: (...a: unknown[]) => replaceMock(...a) }));
vi.mock("../lib/session", () => ({
  refreshTokenStore: { getRefreshToken: () => Promise.resolve("refresh-1"), setRefreshToken },
}));
vi.mock("../components/PinPrompt", () => ({
  PinPrompt: ({ onVerified, initialError }: any) => (
    <div>
      <span data-testid="pin-error">{initialError ?? ""}</span>
      <button type="button" onClick={() => onVerified("proof")}>enter-pin</button>
    </div>
  ),
}));
const authState = { profileId: "prf-adult", accessToken: "t", setTokens, setProfile };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@tanstack/react-query", async (orig) => ({
  ...(await orig<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: () => Promise.resolve() }),
}));
vi.mock("@evey/api", async (orig) => ({
  ...(await orig<typeof import("@evey/api")>()),
  getBaseURL: () => "http://server",
  useUserServiceGetCurrentUser: () => ({ data: { id: "usr-1" } }),
  useUserServiceListProfiles: () => listProfiles(),
  userServiceRefreshToken: (...a: unknown[]) => refreshToken(...a),
}));

import { ProfileSelectScreen } from "./ProfileSelect";

const PROFILES = [
  { id: "prf-adult", name: "Gravity", isDefault: true },
  { id: "prf-kid", name: "Locked", isKids: true },
];

beforeEach(() => {
  refreshToken.mockReset();
  replaceMock.mockReset();
  listProfiles.mockReturnValue({ data: { profiles: PROFILES }, isLoading: false });
});

describe("who is watching", () => {
  it("switches on the server, not on a client flag, and keeps the rotated refresh token", async () => {
    refreshToken.mockResolvedValue({ accessToken: "new-access", refreshToken: "rotated" });
    render(<ProfileSelectScreen />);
    fireEvent.click(screen.getByText("Locked"));
    await waitFor(() => expect(setProfile).toHaveBeenCalledWith("prf-kid"));
    expect(refreshToken).toHaveBeenCalledWith({ profileId: "prf-kid", refreshToken: "refresh-1", pinToken: undefined });
    expect(setRefreshToken).toHaveBeenCalledWith("rotated");
    expect(replaceMock).toHaveBeenCalledWith({ name: "home" });
  });

  it("asks for the PIN when the server refuses, then switches with the proof", async () => {
    refreshToken.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { httpStatus: 403 }));
    render(<ProfileSelectScreen />);
    fireEvent.click(screen.getByText("Locked"));
    await waitFor(() => expect(screen.getByText("enter-pin")).toBeTruthy());
    expect(screen.getByTestId("pin-error").textContent).toBe("");

    refreshToken.mockResolvedValueOnce({ accessToken: "a", refreshToken: "r" });
    fireEvent.click(screen.getByText("enter-pin"));
    await waitFor(() => expect(refreshToken).toHaveBeenLastCalledWith({ profileId: "prf-kid", refreshToken: "refresh-1", pinToken: "proof" }));
  });

  it("a proof the server refuses reopens the prompt saying so", async () => {
    refreshToken.mockRejectedValue(Object.assign(new Error("forbidden"), { httpStatus: 403 }));
    render(<ProfileSelectScreen />);
    fireEvent.click(screen.getByText("Locked"));
    await waitFor(() => expect(screen.getByText("enter-pin")).toBeTruthy());
    fireEvent.click(screen.getByText("enter-pin"));
    await waitFor(() => expect(screen.getByTestId("pin-error").textContent).toBe("Incorrect PIN"));
  });

  it("says something went wrong for anything that is not a refusal", async () => {
    refreshToken.mockRejectedValue(new Error("network down"));
    render(<ProfileSelectScreen />);
    fireEvent.click(screen.getByText("Locked"));
    await waitFor(() => expect(screen.getByText("Something went wrong.")).toBeTruthy());
  });
});
