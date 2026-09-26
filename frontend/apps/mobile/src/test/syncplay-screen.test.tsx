import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, accessibilityLabel, placeholder, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
      ...p,
    }),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  ActivityIndicator: (p: any) =>
    React.createElement("div", { "data-testid": "loading", role: "progressbar", ...p }),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Platform: { OS: "android", select: (o: any) => o.android },
}));

// --- Expo vector icons ----------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- expo-router ---------------------------------------------------------------------
const replaceMock = vi.fn();
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) => React.createElement("div", { "data-testid": "stack-screen", title: options?.title }),
  },
  useRouter: () => ({ replace: replaceMock, back: vi.fn(), push: vi.fn() }),
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
      border: "#333",
      error: "#e00",
      warning: "#fa0",
      success: "#0f0",
    },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

// --- useSyncPlay mock ----------------------------------------------------------------
const joinByCodeMock = vi.fn();
vi.mock("../hooks/useSyncPlay", () => ({
  useSyncPlay: () => ({
    joinByCode: joinByCodeMock,
    state: { connected: false, roomId: null },
  }),
}));

// -------------------------------------------------------------------------------------
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib-music", type: "LIBRARY_TYPE_MUSIC" }] } }),
}));
vi.mock("../stores/musicQueue", () => ({ useMusicQueueStore: { setState: vi.fn() } }));
const builder = vi.hoisted(() => ({ onCreate: null as null | ((items: { id: string; title: string; artist: string }[]) => void) }));
vi.mock("../features/syncplay/RoomBuilder", () => ({
  RoomBuilder: (p: { onCreate: (items: { id: string; title: string; artist: string }[]) => void }) => { builder.onCreate = p.onCreate; return React.createElement("div", { "data-testid": "room-builder" }); },
}));
vi.mock("../features/syncplay/ListenTogetherRoom", () => ({
  ListenTogetherRoom: () => React.createElement("div", { "data-testid": "listen-room" }),
}));
import SyncPlayScreen from "../../app/(auth)/syncplay";
import { Alert } from "react-native";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SyncPlayScreen", () => {
  it("renders title, description and code input", () => {
    render(<SyncPlayScreen />);

    // Title from translation key fallback
    expect(screen.getByText("Watch Together")).toBeTruthy();
    // Description text (partial)
    expect(
      screen.getByText(
        "Got a code from a friend? Enter it below to join their room. To start a new room, open a movie or song and tap the SyncPlay button.",
      ),
    ).toBeTruthy();
    // Input with placeholder
    expect(screen.getByPlaceholderText("ABC123")).toBeTruthy();
  });

  it("Join button is disabled when code length < 5", () => {
    render(<SyncPlayScreen />);

    const joinBtn = screen.getByRole("button", { name: "Join" });
    // No input → button is disabled (disabled attribute present)
    expect(joinBtn.hasAttribute("disabled")).toBe(true);

    // Type 4 chars - still disabled
    fireEvent.change(screen.getByPlaceholderText("ABC123"), {
      target: { value: "1234" },
    });
    expect(screen.getByRole("button", { name: "Join" }).hasAttribute("disabled")).toBe(true);
  });

  it("Join button is enabled once the code is complete", () => {
    render(<SyncPlayScreen />);

    fireEvent.change(screen.getByPlaceholderText("ABC123"), {
      target: { value: "ABC123" },
    });
    expect(screen.getByRole("button", { name: "Join" }).hasAttribute("disabled")).toBe(false);
  });

  it("calls joinByCode once with the normalized code", async () => {
    joinByCodeMock.mockResolvedValue({ roomId: "room-1", mediaId: "media-1" });

    render(<SyncPlayScreen />);

    fireEvent.change(screen.getByPlaceholderText("ABC123"), {
      target: { value: "ABC123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Join" }));

    await waitFor(() => expect(joinByCodeMock).toHaveBeenCalledTimes(1));
    expect(joinByCodeMock).toHaveBeenCalledWith("ABC123");
  });

  it("navigates to /player/<mediaId> on successful join", async () => {
    joinByCodeMock.mockResolvedValue({ roomId: "room-1", mediaId: "media-42" });

    render(<SyncPlayScreen />);

    fireEvent.change(screen.getByPlaceholderText("ABC123"), {
      target: { value: "ABC123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Join" }));

    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith("/player/media-42"));
  });

  it("calls Alert.alert and does not navigate on rejected join", async () => {
    joinByCodeMock.mockRejectedValue(new Error("room not found"));

    render(<SyncPlayScreen />);

    fireEvent.change(screen.getByPlaceholderText("ABC123"), {
      target: { value: "ABC123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Join" }));

    await waitFor(() => expect((Alert.alert as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(0));
    expect(replaceMock).not.toHaveBeenCalled();
  });
});
