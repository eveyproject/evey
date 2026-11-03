import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, waitFor, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", p, "loading"),
  StyleSheet: { create: (s: any) => s },
}));

const replaceMock = vi.fn();
// Leaving the screen pops back to the (auth) layout already on the stack
// (dismissTo), never a fresh copy of it beside the old one.
const dismissToMock = vi.fn();
const searchParams: { pick?: string } = {};
vi.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => replaceMock(...args), dismissTo: (...args: unknown[]) => dismissToMock(...args) },
  useLocalSearchParams: () => searchParams,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", accent: "#f00", onAccent: "#fff", border: "#333" },
  }),
}));

vi.mock("../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
}));

vi.mock("expo-image", () => ({
  Image: (props: any) =>
    React.createElement("img", {
      alt: "",
      src: props.source?.uri,
      "data-headers": props.source?.headers ? JSON.stringify(props.source.headers) : undefined,
    }),
}));

vi.mock("../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));

// --- ParentalPINModal mock ---------------------------------------------------
const pinModalPropsRef: { current: any } = { current: null };
vi.mock("../src/components/ParentalPINModal", () => ({
  ParentalPINModal: (props: any) => {
    pinModalPropsRef.current = props;
    return props.visible
      ? React.createElement(
          "button",
          { "aria-label": "confirm-pin", onClick: () => props.onVerified("proof-token") },
          "confirm-pin",
        )
      : null;
  },
}));

const clearUnlockedMock = vi.fn();
vi.mock("../src/hooks/useParentalGate", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/hooks/useParentalGate")>()),
  clearUnlocked: () => clearUnlockedMock(),
}));

const invalidateQueriesMock = vi.fn();
const clearQueriesMock = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock, clear: clearQueriesMock }),
}));

// The switch sends the stored refresh token and writes the rotated one back
// (SecureStore in the app): the keystore is faked here.
const setRefreshTokenMock = vi.fn(async (_token: string) => {});
vi.mock("../src/lib/refresh-token", () => ({
  getRefreshToken: async () => "stored-rt",
  setRefreshToken: (token: string) => setRefreshTokenMock(token),
  clearRefreshToken: async () => {},
}));

const currentUserMock = vi.fn();
const listProfilesMock = vi.fn();
const refreshTokenMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => currentUserMock(...args),
  useUserServiceListProfiles: (...args: unknown[]) => listProfilesMock(...args),
  userServiceRefreshToken: (...args: unknown[]) => refreshTokenMock(...args),
}));

const setProfileMock = vi.fn();
const setTokensMock = vi.fn();
const toastErrorMock = vi.fn();
let storeState: any;
// Spread the real module: listing only what is used today makes this file go
// red the day the screen imports one more export, on an error that says
// nothing about the change being tested.
vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  useAuthStore: Object.assign(
    (sel: any) => sel(storeState),
    { getState: () => storeState },
  ),
  toast: { error: (...args: unknown[]) => toastErrorMock(...args) },
}));

import ProfileSelect from "./profile-select";
import { classifySwitchFailure } from "./profile-select";

beforeEach(() => {
  vi.clearAllMocks();
  pinModalPropsRef.current = null;
  storeState = {
    setProfile: setProfileMock,
    setTokens: setTokensMock,
    profileId: "prof-kids",
    accessToken: "current-access-token",
  };
  currentUserMock.mockReturnValue({ data: { id: "user-1" } });
  delete searchParams.pick;
});

describe("classifySwitchFailure (pure decision)", () => {
  it("asks for a PIN on a bare 403 (no pinToken was sent)", () => {
    expect(classifySwitchFailure({ httpStatus: 403 }, false)).toBe("pin");
  });
  it("treats a 403 that already carried a pinToken as a hard failure", () => {
    expect(classifySwitchFailure({ httpStatus: 403 }, true)).toBe("fail");
  });
  it("treats any other status as a hard failure", () => {
    expect(classifySwitchFailure({ httpStatus: 500 }, false)).toBe("fail");
  });
  it("treats a network error with no httpStatus as a hard failure", () => {
    expect(classifySwitchFailure(new Error("network down"), false)).toBe("fail");
  });
});

describe("ProfileSelect (TV) - profile switch is server-authorised", () => {
  it("calls RefreshToken with the target profileId and only navigates once the server confirms", async () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    refreshTokenMock.mockResolvedValueOnce({ accessToken: "new-access-token", refreshToken: "rotated-rt" });

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));

    // The stored refresh token travels in the body (the silent refresh of this
    // app sends no cookie, so the jar held a consumed token), and the rotated
    // one is written back so the next silent refresh does not replay it.
    await waitFor(() => expect(refreshTokenMock).toHaveBeenCalledWith({ refreshToken: "stored-rt", profileId: "prof-adult" }));
    expect(setTokensMock).toHaveBeenCalledWith("new-access-token");
    await waitFor(() => expect(setRefreshTokenMock).toHaveBeenCalledWith("rotated-rt"));
    // Every screen under the picker refetches as the new profile, kids filter included.
    expect(clearQueriesMock).toHaveBeenCalledTimes(1);
    // The unlocks of the profile left behind do not follow the viewer.
    expect(clearUnlockedMock).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(dismissToMock).toHaveBeenCalledWith("/(auth)/(tabs)"));
  });

  // A parental unlock belongs to the viewer who typed the PIN: carried over
  // the switch, a kids profile picked next would open the gated titles.
  it("forgets every parental unlock once the server confirmed the switch", async () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    refreshTokenMock.mockResolvedValueOnce({ accessToken: "new-access-token" });

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));

    await waitFor(() => expect(dismissToMock).toHaveBeenCalledWith("/(auth)/(tabs)"));
    expect(clearUnlockedMock).toHaveBeenCalledTimes(1);
  });

  it("keeps the unlocks when the server refuses the switch", async () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    refreshTokenMock.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { httpStatus: 403 }));

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));

    await waitFor(() => expect(screen.getByLabelText("confirm-pin")).toBeTruthy());
    expect(clearUnlockedMock).not.toHaveBeenCalled();
  });

  it("never applies the switch locally when the server rejects with 403 - it opens the PIN gate instead", async () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    refreshTokenMock.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { httpStatus: 403 }));

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));

    await waitFor(() => expect(screen.getByLabelText("confirm-pin")).toBeTruthy());
    expect(clearUnlockedMock).not.toHaveBeenCalled();
    // Critically: no local profile switch happened while the PIN is pending.
    expect(setTokensMock).not.toHaveBeenCalled();
    expect(dismissToMock).not.toHaveBeenCalledWith("/(auth)/(tabs)");
  });

  it("retries RefreshToken with the pinToken once the PIN is verified, then navigates", async () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    refreshTokenMock
      .mockRejectedValueOnce(Object.assign(new Error("forbidden"), { httpStatus: 403 }))
      .mockResolvedValueOnce({ accessToken: "new-access-token" });

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));
    await waitFor(() => expect(screen.getByLabelText("confirm-pin")).toBeTruthy());

    fireEvent.click(screen.getByLabelText("confirm-pin"));

    await waitFor(() =>
      expect(refreshTokenMock).toHaveBeenNthCalledWith(2, { refreshToken: "stored-rt", profileId: "prof-adult", pinToken: "proof-token" }),
    );
    expect(setTokensMock).toHaveBeenCalledWith("new-access-token");
    await waitFor(() => expect(dismissToMock).toHaveBeenCalledWith("/(auth)/(tabs)"));
  });

  it("passes the current (source) profile as fallback so the kids profile's own PIN can authorise the switch", async () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    refreshTokenMock.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { httpStatus: 403 }));

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));

    await waitFor(() => expect(pinModalPropsRef.current).not.toBeNull());
    expect(pinModalPropsRef.current.profileId).toBe("prof-adult");
    expect(pinModalPropsRef.current.fallbackProfileId).toBe("prof-kids");
  });
});
// A profile whose name is an empty string slips past `??` (only null and
// undefined trigger it) and ""[0] is undefined - calling toUpperCase on that
// took the whole picker down, on the one screen a viewer cannot skip.
describe("ProfileSelect survives a nameless profile", () => {
  it("renders the picker instead of crashing, and falls back to a placeholder initial", () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [{ id: "prof-1", name: "" }, { id: "prof-2", name: "Alice" }] },
      isLoading: false,
    });
    render(<ProfileSelect />);
    expect(screen.getByText("?")).toBeTruthy();
    expect(screen.getByText("A")).toBeTruthy();
  });
});

// The TV was the only client that never rendered a profile's avatar image,
// falling back to an initial letter even when the profile had one.
describe("ProfileSelect renders avatars like the other clients", () => {
  it("renders the avatar image with the token in the Authorization header, not the URL", () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true, avatarPath: "avatars/kid.png" },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    render(<ProfileSelect />);
    const img = screen.getByAltText("") as HTMLImageElement;
    expect(img.src).toBe("http://srv.test/api/v1/profiles/prof-kids/avatar");
    expect(img.src).not.toContain("token=");
    expect(img.getAttribute("data-headers")).toBe(
      JSON.stringify({ Authorization: "Bearer current-access-token" }),
    );
  });

  it("falls back to the initial when avatarPath is absent", () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    render(<ProfileSelect />);
    expect(screen.queryByAltText("")).toBeNull();
    expect(screen.getByText("K")).toBeTruthy();
  });
});

// A shared television needs to know at a glance which profile is a kids
// profile and which one is currently active - the whole point of this screen.
describe("ProfileSelect surfaces kids and active markers", () => {
  it("shows the kids badge only on the kids profile", () => {
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    render(<ProfileSelect />);
    expect(screen.getByText("Kids")).toBeTruthy();
  });

  it("marks the currently active profile", () => {
    // storeState.profileId is "prof-kids" (see beforeEach).
    listProfilesMock.mockReturnValue({
      data: { profiles: [
        { id: "prof-kids", name: "Kid", isKids: true },
        { id: "prof-adult", name: "Adult", isKids: false },
      ] },
      isLoading: false,
    });
    render(<ProfileSelect />);
    expect(screen.getByText("Active")).toBeTruthy();
  });
});

// Two adult profiles and no kids one: the walk-through left the header's
// avatar and Settings' "switch profile" unable to switch at all.
describe("ProfileSelect when the viewer asks to switch", () => {
  const ADULTS = { data: { profiles: [{ id: "prof-a", name: "Alice" }, { id: "prof-b", name: "Bob" }] }, isLoading: false, isError: false };

  it("walks through on its own when nobody asked", async () => {
    listProfilesMock.mockReturnValue(ADULTS);
    render(<ProfileSelect />);
    await waitFor(() => expect(dismissToMock).toHaveBeenCalledWith("/(auth)/(tabs)"));
  });

  it("shows the picker when asked, even with no kids profile", () => {
    searchParams.pick = "1";
    listProfilesMock.mockReturnValue(ADULTS);
    render(<ProfileSelect />);
    expect(screen.getByText("Bob")).toBeTruthy();
    expect(dismissToMock).not.toHaveBeenCalled();
  });
});
