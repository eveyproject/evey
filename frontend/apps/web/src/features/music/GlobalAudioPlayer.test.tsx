import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { GlobalAudioPlayer } from "./GlobalAudioPlayer";
import { useAuthStore, useMusicPlayerStore } from "@evey/store";

// We're testing that the <audio src> attribute does not rebuild when
// the Zustand accessToken rotates — the bug that aborted music playback
// every 15 minutes (fix: useMemo keyed on playing.ID, reads via getState).

function makeJwt(expSeconds: number) {
  const header = btoa(JSON.stringify({ alg: "none", typ: "JWT" }));
  const payload = btoa(JSON.stringify({ exp: expSeconds, sub: "u" }));
  return `${header}.${payload}.sig`;
}

beforeEach(() => {
  // reset both stores between tests
  useAuthStore.setState({ accessToken: null, refreshToken: null });
  useMusicPlayerStore.setState({
    playing: null,
    queue: [],
    shuffle: false,
    externallyPaused: false,
  });
  // jsdom doesn't implement HTMLMediaElement.play — stub to avoid unhandled rejections
  window.HTMLMediaElement.prototype.play = vi.fn(() => Promise.resolve());
  window.HTMLMediaElement.prototype.pause = vi.fn();
});

describe("GlobalAudioPlayer", () => {
  it("renders nothing when no track is playing", () => {
    const { container } = render(<GlobalAudioPlayer />);
    expect(container.firstChild).toBeNull();
  });

  it("embeds access token in audio src on track start", () => {
    useAuthStore.setState({ accessToken: makeJwt(9999999999) });
    useMusicPlayerStore.setState({
      playing: {
        ID: "track-1",
        Title: "Test",
        Artist: "Artist",
        Album: "",
        AlbumArtist: "",
        Year: 0,
        TrackNumber: 0,
        DiskNumber: 0,
        Duration: 0,
        FilePath: "",
        MimeType: "",
        Genre: "",
        CoverArtURL: "",
      },
      queue: [],
      shuffle: false,
      externallyPaused: false,
    });

    render(<GlobalAudioPlayer />);
    const audio = document.querySelector("audio");
    expect(audio).not.toBeNull();
    expect(audio?.getAttribute("src")).toContain("/api/v1/music/tracks/track-1/stream?token=");
  });

  it("does NOT rebuild src when the access token rotates mid-playback", () => {
    const initialToken = makeJwt(9999999999);
    useAuthStore.setState({ accessToken: initialToken });
    useMusicPlayerStore.setState({
      playing: {
        ID: "track-1",
        Title: "Test",
        Artist: "",
        Album: "",
        AlbumArtist: "",
        Year: 0,
        TrackNumber: 0,
        DiskNumber: 0,
        Duration: 0,
        FilePath: "",
        MimeType: "",
        Genre: "",
        CoverArtURL: "",
      },
      queue: [],
      shuffle: false,
      externallyPaused: false,
    });
    render(<GlobalAudioPlayer />);
    const audio = document.querySelector("audio");
    const before = audio?.getAttribute("src");
    expect(before).toBeTruthy();

    // Simulate a token refresh — subscribers to accessToken would see the change.
    useAuthStore.setState({ accessToken: makeJwt(1234567890) });

    // Crucial: src must NOT change, otherwise the browser would abort the
    // in-flight audio stream and attempt to reload from byte 0.
    const after = audio?.getAttribute("src");
    expect(after).toBe(before);
  });

  it("rebuilds src when the track changes (keyed by playing.ID)", () => {
    useAuthStore.setState({ accessToken: makeJwt(9999999999) });
    useMusicPlayerStore.setState({
      playing: {
        ID: "track-1",
        Title: "",
        Artist: "",
        Album: "",
        AlbumArtist: "",
        Year: 0,
        TrackNumber: 0,
        DiskNumber: 0,
        Duration: 0,
        FilePath: "",
        MimeType: "",
        Genre: "",
        CoverArtURL: "",
      },
      queue: [],
      shuffle: false,
      externallyPaused: false,
    });
    const { rerender } = render(<GlobalAudioPlayer />);
    expect(document.querySelector("audio")?.getAttribute("src")).toContain("track-1");

    useMusicPlayerStore.setState({
      playing: {
        ID: "track-2",
        Title: "",
        Artist: "",
        Album: "",
        AlbumArtist: "",
        Year: 0,
        TrackNumber: 0,
        DiskNumber: 0,
        Duration: 0,
        FilePath: "",
        MimeType: "",
        Genre: "",
        CoverArtURL: "",
      },
      queue: [],
      shuffle: false,
      externallyPaused: false,
    });
    rerender(<GlobalAudioPlayer />);
    expect(document.querySelector("audio")?.getAttribute("src")).toContain("track-2");
  });
});

// Suppress unused-import lint on screen.
void screen;
