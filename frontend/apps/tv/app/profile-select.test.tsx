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
vi.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => replaceMock(...args), dismissTo: (...args: unknown[]) => dismissToMock(...args) },
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

const invalidateQueriesMock = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
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
    refreshTokenMock.mockResolvedValueOnce({ accessToken: "new-access-token" });

    render(<ProfileSelect />);
    fireEvent.click(screen.getByText("Adult"));

    await waitFor(() => expect(refreshTokenMock).toHaveBeenCalledWith({ profileId: "prof-adult", pinToken: undefined }));
    expect(setTokensMock).toHaveBeenCalledWith("new-access-token");
    await waitFor(() => expect(dismissToMock).toHaveBeenCalledWith("/(auth)/(tabs)"));
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
      expect(refreshTokenMock).toHaveBeenNthCalledWith(2, { profileId: "prof-adult", pinToken: "proof-token" }),
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
    expect(screen.getByText("R")).toBeTruthy();
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

