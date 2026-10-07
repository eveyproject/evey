import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { DEFAULT_SUBTITLE_STYLE } from "../../lib/subtitleStyle";

const eveyFetchRawMock = vi.fn();

// The component reads BYTES now, not text: the same route answers WebVTT for a
// text track and a binary image blob for a bitmap one, and decoding the second
// as UTF-8 would corrupt it before anything could tell them apart.
function vttResponse(vtt: string) {
  const bytes = new TextEncoder().encode(vtt);
  return { arrayBuffer: () => Promise.resolve(bytes.buffer) };
}
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
}));

// subtitleStyle.ts imports expo-secure-store at module scope for its
// persistence helpers - unused by this component, but the real module
// requires the native runtime and crashes outside it if not stubbed here.
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Text: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("span", { ...rest, "data-testid": "subtitle-text", "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Image: ({ source, style, ...rest }: Record<string, unknown>) =>
    React.createElement("img", {
      ...rest,
      "data-testid": "subtitle-image",
      src: (source as { uri?: string })?.uri,
      "data-style": JSON.stringify(style),
    }),
  StyleSheet: { create: (s: Record<string, unknown>) => s, absoluteFill: { position: "absolute" } },
  useWindowDimensions: () => ({ width: 400, height: 1000 }),
}));

import { SubtitleOverlay } from "./SubtitleOverlay";

const VTT = `WEBVTT

00:00:01.000 --> 00:00:03.000
Hello there

00:00:05.000 --> 00:00:07.000
Second line
`;

beforeEach(() => {
  eveyFetchRawMock.mockReset();
});

describe("SubtitleOverlay", () => {
  it("renders nothing when subtitleIndex is null", () => {
    const { container } = render(
      <SubtitleOverlay mediaId="m1" subtitleIndex={null} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    expect(container.innerHTML).toBe("");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  it("fetches the subtitle track and renders the cue active at currentTimeSec", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="m2" subtitleIndex={3} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    const text = await findByTestId("subtitle-text");
    expect(text.textContent).toBe("Hello there");
    expect(eveyFetchRawMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m2/subtitle/3",
      method: "GET",
    });
  });

  it("renders nothing when currentTimeSec falls between cues", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const { container, rerender } = render(
      <SubtitleOverlay mediaId="m3" subtitleIndex={0} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await waitFor(() => expect(container.querySelector("[data-testid='subtitle-text']")).not.toBeNull());

    rerender(
      <SubtitleOverlay mediaId="m3" subtitleIndex={0} currentTimeSec={4} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    expect(container.querySelector("[data-testid='subtitle-text']")).toBeNull();
  });

  it("applies the resync offset before picking the active cue", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    // Second cue is 5-7s; a -1000ms offset (subtitles earlier) shifts it to
    // 4-6s, so currentTimeSec=4.5 now falls inside it.
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="m4" subtitleIndex={0} currentTimeSec={4.5} offsetMs={-1000} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    const text = await findByTestId("subtitle-text");
    expect(text.textContent).toBe("Second line");
  });

  it("applies the style's text color and background", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const style = { ...DEFAULT_SUBTITLE_STYLE, color: "#fde047", bgColor: "#1e3a5f", bgOpacity: 50 };
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="m5" subtitleIndex={0} currentTimeSec={2} style={style} />,
    );
    const text = await findByTestId("subtitle-text");
    const styleAttr = text.getAttribute("data-style");
    expect(styleAttr).toContain("#fde047");
    expect(styleAttr).toContain("rgba(30,58,95,0.5)");
  });

  it("renders externalCues directly, without fetching", () => {
    const externalCues = [{ start: 1, end: 3, text: "External line" }];
    const { getByTestId } = render(
      <SubtitleOverlay
        mediaId="m6"
        subtitleIndex={null}
        currentTimeSec={2}
        style={DEFAULT_SUBTITLE_STYLE}
        externalCues={externalCues}
      />,
    );
    expect(getByTestId("subtitle-text").textContent).toBe("External line");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  it("prioritizes externalCues over a server subtitleIndex", () => {
    const externalCues = [{ start: 1, end: 3, text: "External line" }];
    const { getByTestId } = render(
      <SubtitleOverlay
        mediaId="m7"
        subtitleIndex={3}
        currentTimeSec={2}
        style={DEFAULT_SUBTITLE_STYLE}
        externalCues={externalCues}
      />,
    );
    expect(getByTestId("subtitle-text").textContent).toBe("External line");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  it("applies the resync offset to externalCues too", () => {
    const externalCues = [{ start: 5, end: 7, text: "Second line" }];
    // Same -1000ms shift as the fetched-cue offset test: 5-7s becomes 4-6s.
    const { getByTestId } = render(
      <SubtitleOverlay
        mediaId="m8"
        subtitleIndex={null}
        currentTimeSec={4.5}
        offsetMs={-1000}
        style={DEFAULT_SUBTITLE_STYLE}
        externalCues={externalCues}
      />,
    );
    expect(getByTestId("subtitle-text").textContent).toBe("Second line");
  });

  it("caches cues per (mediaId, subtitleIndex) - a second mount does not re-fetch", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const { findByTestId, unmount } = render(
      <SubtitleOverlay mediaId="m-cache" subtitleIndex={0} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await findByTestId("subtitle-text");
    unmount();

    eveyFetchRawMock.mockClear();
    const { findByTestId: findByTestId2 } = render(
      <SubtitleOverlay mediaId="m-cache" subtitleIndex={0} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await findByTestId2("subtitle-text");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });
});

// Une piste bitmap arrive par la MEME route que le texte, mais en blob binaire.
// Le composant doit la reconnaître et dessiner une image positionnée sur le PLAN
// DES SOUS-TITRES - qui n'est pas la résolution vidéo : tous les disques 2160p
// mesurés portent un plan 1920x1080, donc mettre à l'échelle par la vidéo
// décalerait chaque cue d'un facteur deux.
describe("SubtitleOverlay, piste bitmap", () => {
  const MAGIC = "EVEYPGS1";

  function pgsResponse() {
    const png = new Uint8Array([137, 80, 78, 71]);
    const header = {
      screenW: 1920,
      screenH: 1080,
      cues: [{ s: 0, e: 5000, x: 192, y: 972, w: 960, h: 54, o: 0, l: png.length }],
    };
    const hj = new TextEncoder().encode(JSON.stringify(header));
    const out = new Uint8Array(MAGIC.length + 4 + hj.length + png.length);
    for (let i = 0; i < MAGIC.length; i++) out[i] = MAGIC.charCodeAt(i);
    new DataView(out.buffer).setUint32(MAGIC.length, hj.length, false);
    out.set(hj, MAGIC.length + 4);
    out.set(png, MAGIC.length + 4 + hj.length);
    return { arrayBuffer: () => Promise.resolve(out.buffer) };
  }

  it("dessine la cue image à sa place sur le plan des sous-titres", async () => {
    eveyFetchRawMock.mockResolvedValue(pgsResponse());
    render(
      <SubtitleOverlay mediaId="m9" subtitleIndex={0} currentTimeSec={1} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    const img = await screen.findByTestId("subtitle-image");
    expect(img.getAttribute("src")).toMatch(/^data:image\/png;base64,/);
    const style = JSON.parse(img.getAttribute("data-style") ?? "{}");
    // fenêtre 400x1000 (mock), plan 1920x1080 : x=192 -> 40, y=972 -> 900
    expect(style.left).toBeCloseTo(40, 5);
    expect(style.top).toBeCloseTo(900, 5);
    expect(style.width).toBeCloseTo(200, 5);
  });

  it("ne dessine rien hors de la fenêtre temporelle de la cue", async () => {
    eveyFetchRawMock.mockResolvedValue(pgsResponse());
    render(
      <SubtitleOverlay mediaId="m10" subtitleIndex={0} currentTimeSec={9} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await waitFor(() => expect(eveyFetchRawMock).toHaveBeenCalled());
    expect(screen.queryByTestId("subtitle-image")).toBeNull();
  });
  // A beta tester in direct play, 2026-10-05: /subtitle read the whole 9.9 GB film
  // before the first line showed; the packager serves the same track in 6 s
  // segments, about a second each.
  it("reads the packager's segments around the playhead, in the film's own time, and never /subtitle", async () => {
    const base = "/api/v1/media/seg1/hls/";
    const routes: Record<string, string> = {
      [`${base}od_master.m3u8`]: '#EXTM3U\n#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="fr",X-EVEY-TRACK="0",URI="od_sub1.m3u8?v=1"\n',
      [`${base}od_sub1.m3u8?v=1`]: "#EXTM3U\n#EXTINF:6,\nod_sub1_1.vtt?v=1\n#EXTINF:6,\nod_sub1_2.vtt?v=1\n",
      [`${base}od_sub1_1.vtt?v=1`]: "WEBVTT\n\n00:00:01.000 --> 00:00:03.000\nPremier\n",
      [`${base}od_sub1_2.vtt?v=1`]: "WEBVTT\n\n00:00:07.000 --> 00:00:09.000\nSecond\n",
    };
    eveyFetchRawMock.mockImplementation(async ({ url }: { url: string }) => {
      const body = routes[url];
      return body === undefined ? { ok: false, text: async () => "" } : { ok: true, text: async () => body };
    });
    // 2 s of resync: the cue at 7-9 s shows at 9-11 s on screen.
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="seg1" subtitleIndex={0} currentTimeSec={10} offsetMs={2000} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    expect((await findByTestId("subtitle-text")).textContent).toBe("Second");
    const urls = eveyFetchRawMock.mock.calls.map(([req]) => (req as { url: string }).url);
    expect(urls.some((u) => u.includes("/subtitle/"))).toBe(false);
  });
});
