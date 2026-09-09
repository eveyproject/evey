import { describe, expect, it } from "vitest";
import { findActivePGSCue, isPGSBlob, parsePGSBlob, pgsCueDataURI, type PGSCue } from "./pgsCues";

const MAGIC = "EVEYPGS1";

function blob(header: object, images: Uint8Array[]): Uint8Array {
  const hj = new TextEncoder().encode(JSON.stringify(header));
  const total = MAGIC.length + 4 + hj.length + images.reduce((n, i) => n + i.length, 0);
  const out = new Uint8Array(total);
  for (let i = 0; i < MAGIC.length; i++) out[i] = MAGIC.charCodeAt(i);
  new DataView(out.buffer).setUint32(MAGIC.length, hj.length, false);
  out.set(hj, MAGIC.length + 4);
  let off = MAGIC.length + 4 + hj.length;
  for (const im of images) {
    out.set(im, off);
    off += im.length;
  }
  return out;
}

const IMGS = [new Uint8Array([1, 2, 3]), new Uint8Array([4, 5, 6, 7])];
const HEADER = {
  screenW: 1920,
  screenH: 1080,
  cues: [
    { s: 0, e: 1000, x: 10, y: 900, w: 800, h: 60, o: 0, l: 3 },
    { s: 2000, e: 3000, x: 20, y: 100, w: 400, h: 40, o: 3, l: 4, f: true },
  ],
};

describe("parsePGSBlob", () => {
  it("rend les cues et leurs images aux bons décalages", () => {
    const t = parsePGSBlob(blob(HEADER, IMGS));
    expect(t.cues).toHaveLength(2);
    expect(Array.from(t.cues[0].png)).toEqual([1, 2, 3]);
    expect(Array.from(t.cues[1].png)).toEqual([4, 5, 6, 7]);
  });

  // Le plan des sous-titres n'est PAS la résolution vidéo : tous les disques
  // 2160p mesurés portent un plan 1920x1080. Mettre à l'échelle par la vidéo
  // décalerait et redimensionnerait chaque cue d'un facteur deux.
  it("porte le plan des sous-titres, pas celui de la vidéo", () => {
    const t = parsePGSBlob(blob(HEADER, IMGS));
    expect([t.screenW, t.screenH]).toEqual([1920, 1080]);
  });

  // Une piste « forced » mélange couramment cues forcées et normales dans la
  // même piste, donc le drapeau est par cue.
  it("conserve le drapeau forced par cue", () => {
    const t = parsePGSBlob(blob(HEADER, IMGS));
    expect(t.cues[0].forced).toBe(false);
    expect(t.cues[1].forced).toBe(true);
  });

  it("accepte une piste vide - une piste forcée peut n'avoir aucune cue", () => {
    const t = parsePGSBlob(blob({ screenW: 1920, screenH: 1080, cues: [] }, []));
    expect(t.cues).toHaveLength(0);
    expect(t.screenW).toBe(1920);
  });

  // Refuser plutôt que deviner : un décalage hors du corps indexerait ce qui
  // suit, donc dessinerait les pixels de quelqu'un d'autre.
  it("refuse un blob tronqué, étranger, ou dont une cue déborde", () => {
    expect(() => parsePGSBlob(new Uint8Array([1, 2, 3]))).toThrow();
    expect(() => parsePGSBlob(new TextEncoder().encode("WEBVTT\n\n"))).toThrow();
    const bad = blob({ screenW: 1, screenH: 1, cues: [{ s: 0, e: 1, x: 0, y: 0, w: 1, h: 1, o: 0, l: 9999 }] }, IMGS);
    expect(() => parsePGSBlob(bad)).toThrow();
  });
});

describe("isPGSBlob", () => {
  it("distingue le blob du WebVTT, ce qui est le choix de rendu du lecteur", () => {
    expect(isPGSBlob(blob(HEADER, IMGS))).toBe(true);
    expect(isPGSBlob(new TextEncoder().encode("WEBVTT\n\n00:00.000 --> 00:01.000\nsalut\n"))).toBe(false);
    expect(isPGSBlob(new Uint8Array([]))).toBe(false);
  });
});

describe("findActivePGSCue", () => {
  const cues: PGSCue[] = [
    { startMs: 0, endMs: 1000, x: 0, y: 0, w: 1, h: 1, forced: false, png: new Uint8Array() },
    { startMs: 2000, endMs: 3000, x: 0, y: 0, w: 1, h: 1, forced: false, png: new Uint8Array() },
  ];

  it("trouve la cue active et rend -1 dans les trous", () => {
    expect(findActivePGSCue(cues, 500)).toBe(0);
    expect(findActivePGSCue(cues, 2500)).toBe(1);
    expect(findActivePGSCue(cues, 1500)).toBe(-1);
    expect(findActivePGSCue(cues, 9999)).toBe(-1);
  });

  // Le hint accélère la lecture qui avance ; un saut en arrière doit quand même
  // retrouver la bonne cue, sinon le sous-titre reste bloqué après un retour.
  it("retrouve la bonne cue après un saut en arrière malgré le hint", () => {
    expect(findActivePGSCue(cues, 500, 1)).toBe(0);
  });

  it("rend -1 sans cue", () => {
    expect(findActivePGSCue([], 0)).toBe(-1);
  });
});

describe("pgsCueDataURI", () => {
  // Encodeur écrit à la main parce que btoa n'est pas fiable en React Native et
  // qu'il n'y a de Buffer nulle part. Les trois restes modulo 3 sont le piège.
  it("encode en base64, padding compris", () => {
    const uri = (bytes: number[]) =>
      pgsCueDataURI({ startMs: 0, endMs: 1, x: 0, y: 0, w: 1, h: 1, forced: false, png: new Uint8Array(bytes) });
    expect(uri([77, 97, 110])).toBe("data:image/png;base64,TWFu");
    expect(uri([77, 97])).toBe("data:image/png;base64,TWE=");
    expect(uri([77])).toBe("data:image/png;base64,TQ==");
    expect(uri([])).toBe("data:image/png;base64,");
  });
});
