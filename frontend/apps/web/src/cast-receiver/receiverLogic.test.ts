import { describe, it, expect } from "vitest";
import { absoluteWithToken, probeReceiverCaps, rewriteCastMasterUrl, stripWasmAudio, type CanDisplayType } from "./receiverLogic";

// canDisplayType factice : décode ce que `yes` liste (match par sous-chaîne du
// codec string), refuse le reste. La résolution 4K n'est accordée que si
// `fourK` est vrai.
function fakeCDT(yes: string[], fourK = true): CanDisplayType {
  return (_mime, codec, width) => {
    if (width && width > 1920 && !fourK) return false;
    if (!codec) return true;
    return yes.some((y) => codec.startsWith(y));
  };
}

describe("probeReceiverCaps - le receiver se sonde LUI-MÊME (fini les tables par modèle)", () => {
  it("un device moderne remonte hevc/vp9, la 4K et le HDR du système", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1", "hev1", "vp09"]), true);
    expect(caps.videoCodecs).toEqual(["h264", "hevc", "vp9"]);
    expect(caps.maxHeight).toBe(2160);
    expect(caps.hdr).toBe(true);
  });

  it("un device h264-only 1080p SDR retombe au plancher sans jamais mentir", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1"], false), false);
    expect(caps.videoCodecs).toEqual(["h264"]);
    expect(caps.maxHeight).toBe(1080);
    expect(caps.hdr).toBe(false);
  });

  it("le Dolby natif n'est déclaré que si le device le sonde vrai", () => {
    expect(probeReceiverCaps(fakeCDT(["avc1", "ec-3", "ac-3"]), false).nativeDolby).toBe(true);
    expect(probeReceiverCaps(fakeCDT(["avc1"]), false).nativeDolby).toBe(false);
  });
});

describe("rewriteCastMasterUrl - la négociation se fait sur les caps RÉELLES", () => {
  const MASTER = "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=tok&videoCodecs=h264&audioCodecs=aac&containers=mp4&maxHeight=1080";
  const modern = probeReceiverCaps(fakeCDT(["avc1", "hev1", "vp09"]), true);
  const floorSdr = probeReceiverCaps(fakeCDT(["avc1"], false), false);

  it("réécrit les caps vidéo/hauteur/hdr du master avec les valeurs sondées", () => {
    const out = rewriteCastMasterUrl(MASTER, modern, true);
    expect(out).not.toBeNull();
    const u = new URL(out!.url);
    expect(u.searchParams.get("videoCodecs")).toBe("h264,hevc,vp9");
    expect(u.searchParams.get("maxHeight")).toBe("2160");
    expect(u.searchParams.get("hdr")).toBe("1");
    expect(u.searchParams.get("containers")).toBe("mp4");
    expect(u.searchParams.get("token")).toBe("tok"); // le token de cast survit
  });

  it("sans Dolby natif mais avec wasm : audio AAC + wasmAudio=1 (ac3go sur la TV)", () => {
    const out = rewriteCastMasterUrl(MASTER, modern, true)!;
    const u = new URL(out.url);
    expect(u.searchParams.get("audioCodecs")).toBe("aac,mp3,opus,flac");
    expect(u.searchParams.get("wasmAudio")).toBe("1");
    expect(out.wasmAudio).toBe(true);
  });

  it("avec Dolby natif sondé vrai : passthrough eac3/ac3, PAS de wasm", () => {
    const dolby = probeReceiverCaps(fakeCDT(["avc1", "ec-3", "ac-3"]), false);
    const out = rewriteCastMasterUrl(MASTER, dolby, true)!;
    const u = new URL(out.url);
    expect(u.searchParams.get("audioCodecs")).toBe("aac,mp3,opus,flac,eac3,ac3");
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(out.wasmAudio).toBe(false);
  });

  it("sans wasm dispo ni Dolby : AAC transcodé, pas de wasmAudio (le chemin qui marche toujours)", () => {
    const out = rewriteCastMasterUrl(MASTER, floorSdr, false)!;
    const u = new URL(out.url);
    expect(u.searchParams.get("audioCodecs")).toBe("aac,mp3,opus,flac");
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(u.searchParams.has("hdr")).toBe(false);
  });

  it("laisse intactes les URLs qui ne sont pas notre master (direct play, tiers)", () => {
    expect(rewriteCastMasterUrl("https://evey.example/api/v1/media/m1/stream?token=t", modern, true)).toBeNull();
    expect(rewriteCastMasterUrl("https://example.com/other.m3u8", modern, true)).toBeNull();
    expect(rewriteCastMasterUrl("not a url", modern, true)).toBeNull();
  });
});

describe("stripWasmAudio - le repli quand le moteur wasm meurt sur la TV", () => {
  it("retire wasmAudio et garde tout le reste (le serveur re-sert le master AAC muxé)", () => {
    const url = "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=tok&wasmAudio=1&audioCodecs=aac";
    const out = stripWasmAudio(url);
    const u = new URL(out);
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(u.searchParams.get("token")).toBe("tok");
    expect(u.searchParams.get("audioCodecs")).toBe("aac");
  });
});

describe("absoluteWithToken - le receiver peut être hébergé HORS du serveur média", () => {
  // Hébergé sur GitHub Pages (ou tout autre domaine), le receiver résout les
  // URLs de playlist/segments contre le SERVEUR MÉDIA, jamais contre sa propre
  // origine - injectToken (web, même-origine) rend un chemin relatif qui
  // partirait vers github.io et ferait 404.
  it("garde l'origine du serveur média et rafraîchit le token", () => {
    const out = absoluteWithToken(
      "audio_1.m3u8?token=stale",
      "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=fresh",
      "fresh",
    );
    expect(out).toBe("https://evey.example/api/v1/media/m1/hls/audio_1.m3u8?token=fresh");
  });

  it("laisse une URL déjà absolue sur son propre host", () => {
    const out = absoluteWithToken("https://evey.example/seg1.m4s?a=1", "https://other.example/x", "tok");
    const u = new URL(out);
    expect(u.host).toBe("evey.example");
    expect(u.searchParams.get("token")).toBe("tok");
    expect(u.searchParams.get("a")).toBe("1");
  });

  it("sans token, résout absolu sans en inventer un", () => {
    const out = absoluteWithToken("seg1.m4s", "https://evey.example/hls/audio.m3u8", "");
    expect(out).toBe("https://evey.example/hls/seg1.m4s");
  });
});

describe("mixed content - instance Evey en HTTP sur le LAN, receiver en HTTPS", () => {
  // La page receiver est forcément HTTPS (exigence Cast Console). Le player
  // CAF a le droit de charger du MÉDIA http: (doc Google), mais nos fetch JS
  // (master wasm, segments ac3go) sont du mixed content actif : bloqués.
  // Donc : jamais de wasmAudio vers un serveur http: - l'AAC muxé que le
  // player CAF charge nativement est le seul chemin qui marche là-bas.
  const HTTP_MASTER = "http://192.168.1.50:8096/api/v1/media/m1/hls/master.m3u8?token=tok";

  it("ne demande jamais wasmAudio à un serveur http: depuis une page https:", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1", "hev1"]), false);
    const out = rewriteCastMasterUrl(HTTP_MASTER, caps, true, "https:")!;
    const u = new URL(out.url);
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(out.wasmAudio).toBe(false);
    // Le reste de la négociation (vidéo) reste actif : le player CAF, lui,
    // peut charger ce master http.
    expect(u.searchParams.get("videoCodecs")).toBe("h264,hevc");
  });

  it("garde le chemin wasm quand le serveur est https:", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1"]), false);
    const out = rewriteCastMasterUrl(
      "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=tok",
      caps, true, "https:",
    )!;
    expect(out.wasmAudio).toBe(true);
  });
});
