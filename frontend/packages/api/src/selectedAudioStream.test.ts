import { describe, expect, it } from "vitest";
import { pickAudioStream } from "./selectedAudioStream";

// La release de prod du 09-09 : la PREMIERE piste audio est un TrueHD Atmos 7.1
// anglais, et les pistes françaises viennent après. C'est exactement la forme
// qui faisait mentir le panneau.
const STREAMS = [
  { type: "MEDIA_STREAM_TYPE_VIDEO", index: 0, codec: "hevc" },
  { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, codec: "truehd", language: "eng" },
  { type: "MEDIA_STREAM_TYPE_AUDIO", index: 2, codec: "ac3", language: "fre" },
  { type: "MEDIA_STREAM_TYPE_AUDIO", index: 3, codec: "ac3", language: "fre" },
  { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 5, codec: "hdmv_pgs_subtitle" },
];

describe("pickAudioStream", () => {
  it("trouve la piste par son index ABSOLU (mobile / TV)", () => {
    expect(pickAudioStream(STREAMS, { absolute: 3 })?.codec).toBe("ac3");
    expect(pickAudioStream(STREAMS, { absolute: 3 })?.index).toBe(3);
  });

  it("trouve la piste par son ordinal RELATIF aux pistes audio (web)", () => {
    // relatif 0 = TrueHD, 1 = première AC3, 2 = deuxième AC3
    expect(pickAudioStream(STREAMS, { relative: 2 })?.index).toBe(3);
  });

  // Le bug : le panneau montrait toujours la première piste audio du fichier.
  it("ne renvoie PAS la première piste quand une autre est sélectionnée", () => {
    expect(pickAudioStream(STREAMS, { absolute: 2 })?.index).not.toBe(1);
    expect(pickAudioStream(STREAMS, { relative: 1 })?.index).not.toBe(1);
  });

  it("ignore les pistes vidéo et sous-titres dans le comptage relatif", () => {
    // Si la vidéo comptait, relatif 0 la renverrait.
    expect(pickAudioStream(STREAMS, { relative: 0 })?.type).toBe("MEDIA_STREAM_TYPE_AUDIO");
  });

  it("retombe sur la première piste audio quand rien n'est sélectionné", () => {
    expect(pickAudioStream(STREAMS, {})?.index).toBe(1);
  });

  it("retombe sur la première piste audio quand la sélection ne correspond à rien", () => {
    expect(pickAudioStream(STREAMS, { absolute: 99 })?.index).toBe(1);
    expect(pickAudioStream(STREAMS, { relative: 99 })?.index).toBe(1);
  });

  it("ne renvoie rien quand le média n'a aucune piste audio", () => {
    expect(pickAudioStream([{ type: "MEDIA_STREAM_TYPE_VIDEO", index: 0 }], { absolute: 1 })).toBeUndefined();
    expect(pickAudioStream(undefined, { absolute: 1 })).toBeUndefined();
  });
});
