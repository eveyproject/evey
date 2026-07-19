import { describe, it, expect, beforeEach } from "vitest";
import { useMusicPlayerStore, type MusicTrack, type MusicSource } from "./musicPlayer";

function makeTrack(id: string, overrides: Partial<MusicTrack> = {}): MusicTrack {
  return {
    ID: id,
    Title: `Track ${id}`,
    Artist: "Artist",
    Album: "Album",
    AlbumArtist: "AlbumArtist",
    Year: 2024,
    TrackNumber: 1,
    DiskNumber: 1,
    Duration: 180,
    FilePath: `/music/${id}.flac`,
    MimeType: "audio/flac",
    Genre: "Rock",
    CoverArtURL: "",
    ...overrides,
  };
}

const SOURCE_ALBUM: MusicSource = { type: "album", id: "album-1", label: "Best Of" };

function resetStore() {
  useMusicPlayerStore.setState({
    playing: null,
    queue: [],
    shuffle: false,
    externallyPaused: false,
    source: null,
  });
}

beforeEach(() => {
  localStorage.clear();
  resetStore();
});

// ─── initial state ────────────────────────────────────────────────────────────

describe("initial state", () => {
  it("has no playing track, empty queue, not paused", () => {
    const s = useMusicPlayerStore.getState();
    expect(s.playing).toBeNull();
    expect(s.queue).toHaveLength(0);
    expect(s.externallyPaused).toBe(false);
    expect(s.source).toBeNull();
  });
});

// ─── play ─────────────────────────────────────────────────────────────────────

describe("play", () => {
  it("sets playing track", () => {
    const t = makeTrack("t-1");
    useMusicPlayerStore.getState().play(t);
    expect(useMusicPlayerStore.getState().playing).toEqual(t);
  });

  it("replaces queue when provided", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[0], q);
    expect(useMusicPlayerStore.getState().queue).toEqual(q);
  });

  it("keeps existing queue when no queue argument supplied", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[0], q);
    useMusicPlayerStore.getState().play(q[1]);
    expect(useMusicPlayerStore.getState().queue).toEqual(q);
  });

  it("sets source when provided", () => {
    useMusicPlayerStore.getState().play(makeTrack("t-1"), undefined, SOURCE_ALBUM);
    expect(useMusicPlayerStore.getState().source).toEqual(SOURCE_ALBUM);
  });

  it("clears source when omitted (ad-hoc track)", () => {
    useMusicPlayerStore.getState().play(makeTrack("t-1"), undefined, SOURCE_ALBUM);
    useMusicPlayerStore.getState().play(makeTrack("t-2"));
    expect(useMusicPlayerStore.getState().source).toBeNull();
  });

  it("clears source when explicitly null", () => {
    useMusicPlayerStore.getState().play(makeTrack("t-1"), undefined, SOURCE_ALBUM);
    useMusicPlayerStore.getState().play(makeTrack("t-2"), undefined, null);
    expect(useMusicPlayerStore.getState().source).toBeNull();
  });

  it("clears externallyPaused on play", () => {
    useMusicPlayerStore.getState().play(makeTrack("t-1"));
    useMusicPlayerStore.getState().pauseExternal();
    useMusicPlayerStore.getState().play(makeTrack("t-2"));
    expect(useMusicPlayerStore.getState().externallyPaused).toBe(false);
  });
});

// ─── setQueue ─────────────────────────────────────────────────────────────────

describe("setQueue", () => {
  it("replaces the queue", () => {
    const initial = [makeTrack("t-1")];
    useMusicPlayerStore.getState().setQueue(initial);
    expect(useMusicPlayerStore.getState().queue).toEqual(initial);

    const replacement = [makeTrack("t-2"), makeTrack("t-3")];
    useMusicPlayerStore.getState().setQueue(replacement);
    expect(useMusicPlayerStore.getState().queue).toEqual(replacement);
  });

  it("can set an empty queue", () => {
    useMusicPlayerStore.getState().setQueue([makeTrack("t-1")]);
    useMusicPlayerStore.getState().setQueue([]);
    expect(useMusicPlayerStore.getState().queue).toHaveLength(0);
  });
});

// ─── next ─────────────────────────────────────────────────────────────────────

describe("next", () => {
  it("advances to the next track in queue order", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2"), makeTrack("t-3")];
    useMusicPlayerStore.getState().play(q[0], q);

    useMusicPlayerStore.getState().next();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-2");

    useMusicPlayerStore.getState().next();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-3");
  });

  it("does nothing at the last track (no wrap-around)", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[1], q);

    useMusicPlayerStore.getState().next();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-2");
  });

  it("does nothing when queue is empty", () => {
    useMusicPlayerStore.getState().play(makeTrack("t-1"));
    // queue is empty by default when no queue arg given
    useMusicPlayerStore.getState().next();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-1");
  });

  it("does nothing when playing is null", () => {
    useMusicPlayerStore.getState().setQueue([makeTrack("t-1")]);
    useMusicPlayerStore.getState().next();
    expect(useMusicPlayerStore.getState().playing).toBeNull();
  });

  it("clears externallyPaused on next", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[0], q);
    useMusicPlayerStore.getState().pauseExternal();

    useMusicPlayerStore.getState().next();
    expect(useMusicPlayerStore.getState().externallyPaused).toBe(false);
  });

  it("picks a random track from the queue when shuffle is on", () => {
    useMusicPlayerStore.setState({ shuffle: true });
    const q = [makeTrack("t-1"), makeTrack("t-2"), makeTrack("t-3"), makeTrack("t-4")];
    useMusicPlayerStore.getState().play(q[0], q);

    // With 4 tracks, the probability that all 20 calls pick t-1 is negligible.
    const seen = new Set<string>();
    for (let i = 0; i < 20; i++) {
      useMusicPlayerStore.getState().next();
      const playing = useMusicPlayerStore.getState().playing;
      if (playing) seen.add(playing.ID);
    }
    // At least two distinct tracks should have appeared
    expect(seen.size).toBeGreaterThan(1);
  });
});

// ─── prev ─────────────────────────────────────────────────────────────────────

describe("prev", () => {
  it("goes back to the previous track", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2"), makeTrack("t-3")];
    useMusicPlayerStore.getState().play(q[2], q);

    useMusicPlayerStore.getState().prev();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-2");

    useMusicPlayerStore.getState().prev();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-1");
  });

  it("does nothing at the first track", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[0], q);

    useMusicPlayerStore.getState().prev();
    expect(useMusicPlayerStore.getState().playing?.ID).toBe("t-1");
  });

  it("does nothing when playing is null", () => {
    useMusicPlayerStore.getState().setQueue([makeTrack("t-1")]);
    useMusicPlayerStore.getState().prev();
    expect(useMusicPlayerStore.getState().playing).toBeNull();
  });

  it("clears externallyPaused on prev", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[1], q);
    useMusicPlayerStore.getState().pauseExternal();

    useMusicPlayerStore.getState().prev();
    expect(useMusicPlayerStore.getState().externallyPaused).toBe(false);
  });
});

// ─── toggleShuffle ────────────────────────────────────────────────────────────

describe("toggleShuffle", () => {
  it("toggles shuffle on and off", () => {
    expect(useMusicPlayerStore.getState().shuffle).toBe(false);
    useMusicPlayerStore.getState().toggleShuffle();
    expect(useMusicPlayerStore.getState().shuffle).toBe(true);
    useMusicPlayerStore.getState().toggleShuffle();
    expect(useMusicPlayerStore.getState().shuffle).toBe(false);
  });

  it("persists shuffle state to localStorage", () => {
    useMusicPlayerStore.getState().toggleShuffle();
    expect(localStorage.getItem("evey-music-shuffle")).toBe("1");
    useMusicPlayerStore.getState().toggleShuffle();
    expect(localStorage.getItem("evey-music-shuffle")).toBe("0");
  });
});

// ─── pauseExternal / resumeExternal ──────────────────────────────────────────

describe("pauseExternal / resumeExternal", () => {
  it("pauseExternal sets externallyPaused=true", () => {
    useMusicPlayerStore.getState().pauseExternal();
    expect(useMusicPlayerStore.getState().externallyPaused).toBe(true);
  });

  it("resumeExternal sets externallyPaused=false", () => {
    useMusicPlayerStore.getState().pauseExternal();
    useMusicPlayerStore.getState().resumeExternal();
    expect(useMusicPlayerStore.getState().externallyPaused).toBe(false);
  });
});

// ─── close ────────────────────────────────────────────────────────────────────

describe("close", () => {
  it("clears playing, queue, externallyPaused, and source", () => {
    const q = [makeTrack("t-1"), makeTrack("t-2")];
    useMusicPlayerStore.getState().play(q[0], q, SOURCE_ALBUM);
    useMusicPlayerStore.getState().pauseExternal();

    useMusicPlayerStore.getState().close();

    const s = useMusicPlayerStore.getState();
    expect(s.playing).toBeNull();
    expect(s.queue).toHaveLength(0);
    expect(s.externallyPaused).toBe(false);
    expect(s.source).toBeNull();
  });

  it("does not affect shuffle (intentional: shuffle is a user preference)", () => {
    useMusicPlayerStore.setState({ shuffle: true });
    useMusicPlayerStore.getState().close();
    expect(useMusicPlayerStore.getState().shuffle).toBe(true);
  });
});
