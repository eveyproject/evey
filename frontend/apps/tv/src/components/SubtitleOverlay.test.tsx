import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";

const eveyFetchRawMock = vi.fn();

// Le composant lit des OCTETS : la même route répond du WebVTT pour une piste
// texte et un blob binaire pour une piste bitmap, et décoder le second en UTF-8
// le corromprait avant qu'on puisse les distinguer.
function vttResponse(vtt: string) {
  const bytes = new TextEncoder().encode(vtt);
  return { arrayBuffer: () => Promise.resolve(bytes.buffer) };
}

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
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
  useWindowDimensions: () => ({ width: 1920, height: 1080 }),
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

describe("SubtitleOverlay (TV)", () => {
  it("ne rend rien et ne fetche rien quand aucune piste n'est choisie", () => {
    const { container } = render(<SubtitleOverlay mediaId="m1" subtitleIndex={null} currentTimeSec={2} />);
    expect(container.innerHTML).toBe("");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  // La TV ne dessinait AUCUN sous-titre : une piste texte partait au serveur en
  // `?subtitleTrack=`, qui répondait un sidecar que personne n'allait chercher.
  it("dessine la cue texte active, sur l'ordinal RELATIF aux sous-titres", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const { findByTestId } = render(<SubtitleOverlay mediaId="m2" subtitleIndex={1} currentTimeSec={2} />);
    expect((await findByTestId("subtitle-text")).textContent).toBe("Hello there");
    expect(eveyFetchRawMock).toHaveBeenCalledWith({ url: "/api/v1/media/m2/subtitle/1", method: "GET" });
  });

  it("ne rend rien entre deux cues", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const { container, rerender } = render(<SubtitleOverlay mediaId="m3" subtitleIndex={0} currentTimeSec={2} />);
    await waitFor(() => expect(container.querySelector("[data-testid='subtitle-text']")).not.toBeNull());
    rerender(<SubtitleOverlay mediaId="m3" subtitleIndex={0} currentTimeSec={4} />);
    expect(container.querySelector("[data-testid='subtitle-text']")).toBeNull();
  });

  it("met en cache par (media, piste) - un remontage ne refetche pas", async () => {
    eveyFetchRawMock.mockResolvedValue(vttResponse(VTT));
    const { findByTestId, unmount } = render(<SubtitleOverlay mediaId="m-cache" subtitleIndex={0} currentTimeSec={2} />);
    await findByTestId("subtitle-text");
    unmount();

    eveyFetchRawMock.mockClear();
    const { findByTestId: again } = render(<SubtitleOverlay mediaId="m-cache" subtitleIndex={0} currentTimeSec={2} />);
    await again("subtitle-text");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });
});

// Une piste bitmap arrive par la MEME route, en blob binaire. La mise à
// l'échelle se fait sur le PLAN DES SOUS-TITRES, qui n'est pas la résolution
// vidéo : tous les disques 2160p mesurés portent un plan 1920x1080, donc passer
// par les dimensions du média décalerait chaque cue d'un facteur deux.
describe("SubtitleOverlay (TV), piste bitmap", () => {
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

  it("dessine la cue image à sa place, sans incrustation serveur", async () => {
    eveyFetchRawMock.mockResolvedValue(pgsResponse());
    render(<SubtitleOverlay mediaId="m9" subtitleIndex={0} currentTimeSec={1} />);
    const img = await screen.findByTestId("subtitle-image");
    expect(img.getAttribute("src")).toMatch(/^data:image\/png;base64,/);
    // fenêtre 1920x1080 (mock) = plan 1920x1080, donc échelle 1:1
    const style = JSON.parse(img.getAttribute("data-style") ?? "{}");
    expect(style.left).toBeCloseTo(192, 5);
    expect(style.top).toBeCloseTo(972, 5);
    expect(style.width).toBeCloseTo(960, 5);
    expect(style.height).toBeCloseTo(54, 5);
  });

  it("ne dessine rien hors de la fenêtre temporelle de la cue", async () => {
    eveyFetchRawMock.mockResolvedValue(pgsResponse());
    const { container } = render(<SubtitleOverlay mediaId="m10" subtitleIndex={0} currentTimeSec={1} />);
    await waitFor(() => expect(container.querySelector("[data-testid='subtitle-image']")).not.toBeNull());
    // la cue couvre 0-5000ms
    const { container: later } = render(<SubtitleOverlay mediaId="m10" subtitleIndex={0} currentTimeSec={9} />);
    expect(later.querySelector("[data-testid='subtitle-image']")).toBeNull();
  });
});
