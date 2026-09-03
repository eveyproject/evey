import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { GlobalAudioPlayer } from "./GlobalAudioPlayer";
import { useAuthStore, useMusicPlayerStore } from "@evey/store";
import { eveyFetchRaw } from "@evey/api";

const { mockNavigate } = vi.hoisted(() => ({ mockNavigate: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mockNavigate }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()), eveyFetchRaw: vi.fn(() => Promise.resolve(new Response(null, { status: 204 }))) }));

// We're testing that the <audio src> attribute does not rebuild when
// the Zustand accessToken rotates - the bug that aborted music playback
// every 15 minutes (fix: useMemo keyed on playing.ID, reads via getState).

function makeJwt(expSeconds: number) {
  const header = btoa(JSON.stringify({ alg: "none", typ: "JWT" }));
  const payload = btoa(JSON.stringify({ exp: expSeconds, sub: "u" }));
  return `${header}.${payload}.sig`;
}

function makeTrack(id: string, title: string, artist = "") {
  return {
    ID: id, Title: title, Artist: artist, Album: "", AlbumArtist: "", Year: 0,
    TrackNumber: 0, DiskNumber: 0, Duration: 0, FilePath: "", MimeType: "", Genre: "", CoverArtURL: "",
  };
}

beforeEach(() => {
  mockNavigate.mockClear();
  // reset both stores between tests
  useAuthStore.setState({ accessToken: null });
  useMusicPlayerStore.setState({
    playing: null,
    queue: [],
    shuffle: false,
    externallyPaused: false,
    source: null,
  });
  // jsdom doesn't implement HTMLMediaElement.play - stub to avoid unhandled rejections
  window.HTMLMediaElement.prototype.play = vi.fn(() => Promise.resolve());
  window.HTMLMediaElement.prototype.pause = vi.fn();
});

// The player starts playback in a mount effect and mirrors the outcome into
// state once `play()` resolves: flush that microtask inside act() so the
// assertions read the settled tree and React stays quiet about it.
async function renderPlayer() {
  const result = render(<GlobalAudioPlayer />);
  await act(async () => {});
  return result;
}

describe("GlobalAudioPlayer", () => {
  it("renders nothing when no track is playing", async () => {
    const { container } = await renderPlayer();
    expect(container.firstChild).toBeNull();
  });

  it("embeds access token in audio src on track start", async () => {
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

    await renderPlayer();
    const audio = document.querySelector("audio");
    expect(audio).not.toBeNull();
    expect(audio?.getAttribute("src")).toContain("/api/v1/music/tracks/track-1/stream?token=");
  });

  it("does NOT rebuild src when the access token rotates mid-playback", async () => {
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
    await renderPlayer();
    const audio = document.querySelector("audio");
    const before = audio?.getAttribute("src");
    expect(before).toBeTruthy();

    // Simulate a token refresh - subscribers to accessToken would see the change.
    useAuthStore.setState({ accessToken: makeJwt(1234567890) });

    // Crucial: src must NOT change, otherwise the browser would abort the
    // in-flight audio stream and attempt to reload from byte 0.
    const after = audio?.getAttribute("src");
    expect(after).toBe(before);
  });

  it("rebuilds src when the track changes (keyed by playing.ID)", async () => {
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
    const { rerender } = await renderPlayer();
    expect(document.querySelector("audio")?.getAttribute("src")).toContain("track-1");

    // The player is mounted and subscribed: a store write re-renders it.
    act(() => useMusicPlayerStore.setState({
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
    }));
    rerender(<GlobalAudioPlayer />);
    await act(async () => {});
    expect(document.querySelector("audio")?.getAttribute("src")).toContain("track-2");
  });

  it("links the track to its album source and navigates without remounting audio", async () => {
    useAuthStore.setState({ accessToken: makeJwt(9999999999) });
    useMusicPlayerStore.setState({
      playing: {
        ID: "track-1", Title: "Song", Artist: "Artist", Album: "Greatest Hits",
        AlbumArtist: "", Year: 0, TrackNumber: 0, DiskNumber: 0, Duration: 0,
        FilePath: "", MimeType: "", Genre: "", CoverArtURL: "",
      },
      queue: [], shuffle: false, externallyPaused: false,
      source: { type: "album", id: "Greatest Hits", label: "Greatest Hits" },
    });
    await renderPlayer();
    fireEvent.click(screen.getByRole("button", { name: "Greatest Hits" }));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/music", search: { tab: "tracks", album: "Greatest Hits" } });
  });

  it("shows a non-interactive title when no source is set", async () => {
    useAuthStore.setState({ accessToken: makeJwt(9999999999) });
    useMusicPlayerStore.setState({
      playing: {
        ID: "track-1", Title: "Song", Artist: "Artist", Album: "",
        AlbumArtist: "", Year: 0, TrackNumber: 0, DiskNumber: 0, Duration: 0,
        FilePath: "", MimeType: "", Genre: "", CoverArtURL: "",
      },
      queue: [], shuffle: false, externallyPaused: false, source: null,
    });
    await renderPlayer();
    expect(screen.queryByRole("button", { name: "Song" })).toBeNull();
  });

  it("records a finished track only once even if `ended` fires twice", async () => {
    (eveyFetchRaw as ReturnType<typeof vi.fn>).mockClear();
    useAuthStore.setState({ accessToken: makeJwt(9999999999) });
    useMusicPlayerStore.setState({
      playing: {
        ID: "track-1", Title: "T", Artist: "A", Album: "", AlbumArtist: "",
        Year: 0, TrackNumber: 0, DiskNumber: 0, Duration: 0, FilePath: "",
        MimeType: "", Genre: "", CoverArtURL: "",
      },
      queue: [], shuffle: false, externallyPaused: false, source: null,
    });

    await renderPlayer();
    const audio = document.querySelector("audio")!;
    fireEvent.loadedMetadata(audio); // new playthrough → arms the guard
    fireEvent.ended(audio);
    fireEvent.ended(audio); // duplicate ended for the same playthrough

    const playedCalls = (eveyFetchRaw as ReturnType<typeof vi.fn>).mock.calls.filter(
      (c) => String((c[0] as { url?: string })?.url).includes("/played"),
    );
    expect(playedCalls).toHaveLength(1);
  });

  describe("queue panel", () => {
    beforeEach(() => {
      useAuthStore.setState({ accessToken: makeJwt(9999999999) });
      useMusicPlayerStore.setState({
        playing: makeTrack("track-1", "Song A", "Artist A"),
        queue: [makeTrack("track-1", "Song A", "Artist A"), makeTrack("track-2", "Song B", "Artist B")],
        shuffle: false,
        externallyPaused: false,
        source: null,
      });
    });

    it("is hidden until the queue button is toggled", async () => {
      await renderPlayer();
      expect(screen.queryByRole("dialog")).toBeNull();
      fireEvent.click(screen.getByRole("button", { name: "Queue" }));
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("lists every queued track and marks the current one", async () => {
      await renderPlayer();
      fireEvent.click(screen.getByRole("button", { name: "Queue" }));

      expect(screen.getByRole("button", { name: /Song A/ })).toHaveAttribute("aria-current", "true");
      expect(screen.getByRole("button", { name: /Song B/ })).not.toHaveAttribute("aria-current");
    });

    it("jumps to a track when its row is clicked", async () => {
      await renderPlayer();
      fireEvent.click(screen.getByRole("button", { name: "Queue" }));
      fireEvent.click(screen.getByRole("button", { name: /Song B/ }));

      expect(useMusicPlayerStore.getState().playing?.ID).toBe("track-2");
      // Jumping must not drop the queue.
      expect(useMusicPlayerStore.getState().queue.map((t) => t.ID)).toEqual(["track-1", "track-2"]);
    });

    it("removes a non-playing track without disturbing playback", async () => {
      await renderPlayer();
      fireEvent.click(screen.getByRole("button", { name: "Queue" }));
      fireEvent.click(screen.getAllByRole("button", { name: "Remove from queue" })[1]);

      expect(useMusicPlayerStore.getState().queue.map((t) => t.ID)).toEqual(["track-1"]);
      expect(useMusicPlayerStore.getState().playing?.ID).toBe("track-1");
    });

    it("removing the currently playing track advances to its neighbour", async () => {
      await renderPlayer();
      fireEvent.click(screen.getByRole("button", { name: "Queue" }));
      fireEvent.click(screen.getAllByRole("button", { name: "Remove from queue" })[0]);

      expect(useMusicPlayerStore.getState().playing?.ID).toBe("track-2");
      expect(useMusicPlayerStore.getState().queue.map((t) => t.ID)).toEqual(["track-2"]);
    });

    it("shows an empty message when the queue has no other tracks", async () => {
      useMusicPlayerStore.setState({ queue: [] });
      await renderPlayer();
      fireEvent.click(screen.getByRole("button", { name: "Queue" }));
      expect(screen.getByText("Queue empty")).toBeInTheDocument();
    });
  });
});

// Suppress unused-import lint on screen.
void screen;
