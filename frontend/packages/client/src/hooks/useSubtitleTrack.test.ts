// a film on a LG television, 2026-10-05: the player fetched /subtitle,
// a read of the whole film paced behind its own playback, three times over;
// the packager served the same tracks segment by segment in 0.3 s each and
// was never asked. These tests pin that a player passing its position reads
// the segments around it and never /subtitle when a rendition exists.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const requested: string[] = [];
let routes: Record<string, string> = {};

vi.mock("@evey/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@evey/api")>();
  return {
    ...actual,
    eveyFetchRaw: vi.fn(async ({ url }: { url: string }) => {
      requested.push(url);
      const body = routes[url];
      if (body === undefined) return new Response("not found", { status: 404 });
      return new Response(body, { status: 200 });
    }),
  };
});

import { useSubtitleTrack } from "./useSubtitleTrack";

function master(tracks: number[]): string {
  return [
    "#EXTM3U",
    ...tracks.map((t) => `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="t${t}",X-EVEY-TRACK="${t}",URI="od_sub${t + 1}.m3u8?v=1"`),
    "#EXT-X-STREAM-INF:BANDWIDTH=1",
    "od_playlist.m3u8?v=1",
  ].join("\n");
}

// Ten 6 s segments; segment i carries one cue in its middle.
function rendition(base: string, track: number, label: string) {
  const lines = ["#EXTM3U", "#EXT-X-TARGETDURATION:6"];
  for (let i = 0; i < 10; i++) {
    const uri = `od_sub${track + 1}_${i}.vtt?v=1`;
    lines.push("#EXTINF:6,", uri);
    const s = i * 6 + 2;
    routes[base + uri] = `WEBVTT\n\n00:00:${String(s).padStart(2, "0")}.000 --> 00:00:${String(s + 2).padStart(2, "0")}.000\n${label} ${i}\n`;
  }
  routes[`${base}od_sub${track + 1}.m3u8?v=1`] = lines.join("\n");
}

beforeEach(() => {
  requested.length = 0;
  routes = {};
});

describe("useSubtitleTrack", () => {
  it("reads the segments around the playhead and never the whole-film route", async () => {
    const base = "/api/v1/media/m1/hls/";
    routes[`${base}od_master.m3u8`] = master([0, 1]);
    rendition(base, 1, "fr");

    const { result } = renderHook(() => useSubtitleTrack("m1", 1, 0));
    await waitFor(() => expect(result.current.cues.length).toBeGreaterThan(0));

    expect(result.current.cues[0].text).toBe("fr 0");
    expect(requested.some((u) => u.includes("/subtitle/"))).toBe(false);
    // Window: 0..70 s covers the whole 60 s rendition here, nothing past it.
    await waitFor(() => expect(result.current.cues).toHaveLength(10));
  });

  it("moves the window with the playhead after a seek", async () => {
    const base = "/api/v1/media/m2/hls/";
    routes[`${base}od_master.m3u8`] = master([0]);
    // A long rendition: 100 segments, so the first window cannot reach the end.
    const lines = ["#EXTM3U"];
    for (let i = 0; i < 100; i++) {
      const uri = `od_sub1_${i}.vtt?v=1`;
      lines.push("#EXTINF:6,", uri);
      const s = i * 6 + 2;
      const ts = (x: number) => `00:${String(Math.floor(x / 60)).padStart(2, "0")}:${String(x % 60).padStart(2, "0")}.000`;
      routes[base + uri] = `WEBVTT\n\n${ts(s)} --> ${ts(s + 2)}\nseg ${i}\n`;
    }
    routes[`${base}od_sub1.m3u8?v=1`] = lines.join("\n");

    const { result, rerender } = renderHook(({ pos }) => useSubtitleTrack("m2", 0, pos), { initialProps: { pos: 0 } });
    await waitFor(() => expect(result.current.cues.length).toBeGreaterThan(0));
    expect(requested).not.toContain(`${base}od_sub1_90.vtt?v=1`);

    rerender({ pos: 540 });
    await waitFor(() => expect(result.current.cues.some((c) => c.text === "seg 90")).toBe(true));
    expect(requested.some((u) => u.includes("/subtitle/"))).toBe(false);
  });

  it("falls back to /subtitle for a track the master does not carry", async () => {
    routes["/api/v1/media/m3/hls/od_master.m3u8"] = master([0]);
    routes["/api/v1/media/m3/subtitle/1"] = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nwhole\n";

    const { result } = renderHook(() => useSubtitleTrack("m3", 1, 0));
    await waitFor(() => expect(result.current.cues.map((c) => c.text)).toEqual(["whole"]));
  });

  it("falls back to /subtitle when the packager answers no master at all", async () => {
    routes["/api/v1/media/m4/subtitle/0"] = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nwhole\n";

    const { result } = renderHook(() => useSubtitleTrack("m4", 0, 0));
    await waitFor(() => expect(result.current.cues.map((c) => c.text)).toEqual(["whole"]));
  });

  it("keeps the whole-film route for a caller that passes no position", async () => {
    routes["/api/v1/media/m5/hls/od_master.m3u8"] = master([0]);
    routes["/api/v1/media/m5/subtitle/0"] = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nwhole\n";

    const { result } = renderHook(() => useSubtitleTrack("m5", 0));
    await waitFor(() => expect(result.current.cues.map((c) => c.text)).toEqual(["whole"]));
    expect(requested.some((u) => u.includes("od_master"))).toBe(false);
  });

  it("does not pour the previous track's cues into the new one", async () => {
    const base = "/api/v1/media/m6/hls/";
    routes[`${base}od_master.m3u8`] = master([0, 1]);
    rendition(base, 0, "eng");
    rendition(base, 1, "fr");

    const { result, rerender } = renderHook(({ track }) => useSubtitleTrack("m6", track, 0), { initialProps: { track: 0 } });
    await waitFor(() => expect(result.current.cues.length).toBeGreaterThan(0));

    rerender({ track: 1 });
    await waitFor(() => expect(result.current.cues).toHaveLength(10));
    expect(result.current.cues.every((c) => c.text.startsWith("fr "))).toBe(true);
  });
});
