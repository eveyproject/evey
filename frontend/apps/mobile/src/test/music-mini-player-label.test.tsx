import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// The whole mini-player carried a hardcoded English accessibilityLabel
// ("Music player") regardless of the app's language.

vi.mock("react-native", () => ({
  View: ({ children, accessibilityLabel, ...p }: any) =>
    React.createElement("div", { "aria-label": accessibilityLabel, ...p }, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  AppState: { currentState: "active", addEventListener: () => ({ remove: vi.fn() }) },
  Platform: { OS: "android", select: (obj: any) => obj.android },
}));

vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));
vi.mock("expo-router", () => ({ router: { push: vi.fn() } }));

const audioPlayer = { replace: vi.fn(), play: vi.fn(), pause: vi.fn(), seekTo: vi.fn() };
vi.mock("expo-audio", () => ({
  useAudioPlayer: () => audioPlayer,
  useAudioPlayerStatus: () => ({ currentTime: 0, duration: 100, playing: true, didJustFinish: false, isLoaded: true }),
}));

const FR_LABELS: Record<string, string> = { "media.music.miniPlayer": "Lecteur de musique" };
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => FR_LABELS[key] ?? key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surfaceRaised: "#111", border: "#333", text: "#fff", textMuted: "#888", accent: "#f00" },
  }),
}));

vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../lib/musicScrobble", () => ({ shouldRecordScrobble: () => false }));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ accessToken: "tok-1" }),
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

const track = { id: "trk-1", title: "Some Track", artist: "Some Artist", album: "Some Album", duration: 100 };
vi.mock("../stores/musicQueue", () => ({
  useMusicQueueStore: (selector: (s: any) => unknown) =>
    selector({
      queue: [track],
      currentIndex: 0,
      isPlaying: true,
      shuffle: false,
      source: null,
      togglePlay: vi.fn(),
      toggleShuffle: vi.fn(),
      next: vi.fn(),
      previous: vi.fn(),
      clear: vi.fn(),
    }),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn().mockResolvedValue({}),
}));

import { MusicMiniPlayer } from "../components/MusicMiniPlayer";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("MusicMiniPlayer - localized root label", () => {
  it("labels the player root using the active language, not a hardcoded string", () => {
    render(<MusicMiniPlayer />);
    expect(screen.getByLabelText("Lecteur de musique")).toBeTruthy();
  });
});
