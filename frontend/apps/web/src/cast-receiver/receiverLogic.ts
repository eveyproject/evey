// Logique PURE du Web Receiver Cast custom : le receiver tourne SUR la TV,
// donc il peut sonder ses propres capacités (canDisplayType) au lieu de faire
// confiance à une table statique par modèle côté sender - la table Mi TV a
// menti une fois (E-AC-3 « certifié Dolby » que le runtime cast ne joue pas,
// prod 2026-07-25) ; un probe local ne peut pas.
//
// Politique audio, alignée sur l'évidence Jellyfin (leur receiver hardcode
// EAC3/AC3 à false sur HLS) ET sur notre incident : le passthrough Dolby n'est
// tenté QUE si le device le sonde vrai lui-même ; sinon l'audio est soit
// transcodé AAC par le serveur, soit décodé SUR LA TV par ac3go wasm
// (wasmAudio=1 - le serveur ne prend ce chemin que si la piste est AC-3/E-AC-3,
// sinon il retombe de lui-même sur le master AAC muxé).

export type CanDisplayType = (
  mimeType: string,
  codec?: string,
  width?: number,
  height?: number,
) => boolean;

export interface ReceiverCaps {
  videoCodecs: string[];
  maxHeight: number;
  hdr: boolean;
  nativeDolby: boolean;
}

// Codec strings de sonde - mêmes formes que le codecSupportHelper de
// jellyfin-chromecast : h264 High L4.0, HEVC Main10 L5.1, VP9 profile 2 10-bit.
const PROBE_H264 = ["video/mp4", "avc1.640028"] as const;
const PROBE_HEVC = ["video/mp4", "hev1.2.4.L153.B0"] as const;
const PROBE_VP9 = ["video/webm", "vp09.02.51.10"] as const;
// HDR10 : HEVC Main10 + transfert PQ (SMPTE ST 2084) via la forme longue vp09,
// la seule syntaxe canDisplayType qui exprime un transfert HDR sans Dolby Vision.
const PROBE_HDR = ["video/webm", "vp09.02.51.10.01.09.16.09.01"] as const;

export function probeReceiverCaps(canDisplayType: CanDisplayType, systemHdr: boolean): ReceiverCaps {
  const videoCodecs: string[] = [];
  // h264 est le plancher universel des receivers : présent même si la sonde
  // du profil High échoue (un receiver sans h264 n'existe pas).
  videoCodecs.push("h264");
  if (canDisplayType(PROBE_HEVC[0], PROBE_HEVC[1])) videoCodecs.push("hevc");
  if (canDisplayType(PROBE_VP9[0], PROBE_VP9[1])) videoCodecs.push("vp9");

  // 4K sondée sur le codec le plus capable disponible ; un échec = plafond 1080p.
  const fourKProbe = videoCodecs.includes("hevc") ? PROBE_HEVC : PROBE_H264;
  const maxHeight = canDisplayType(fourKProbe[0], fourKProbe[1], 3840, 2160) ? 2160 : 1080;

  // HDR : le signal système (DeviceCapabilities.IS_HDR_SUPPORTED, câblé par
  // l'appelant) OU une sonde de transfert PQ positive.
  const hdr = systemHdr || canDisplayType(PROBE_HDR[0], PROBE_HDR[1]);

  // Dolby passthrough : uniquement si le device lui-même le sonde vrai (EDID /
  // réglage Google Home). C'est la sonde que Jellyfin a écrite puis commentée ;
  // notre repli à nous si elle ment est le reload sans wasm/dolby côté receiver.
  const nativeDolby =
    canDisplayType("audio/mp4", "ec-3") && canDisplayType("audio/mp4", "ac-3");

  return { videoCodecs, maxHeight, hdr, nativeDolby };
}

export interface RewrittenLoad {
  url: string;
  wasmAudio: boolean;
}

const BASE_AUDIO = ["aac", "mp3", "opus", "flac"];

// rewriteCastMasterUrl re-négocie NOTRE master HLS avec les caps réellement
// sondées. Toute autre URL (direct play /stream, contenu tiers) est laissée
// telle quelle - retour null.
//
// pageProtocol = le schéma de la PAGE receiver (https: en prod, exigence Cast
// Console). Une instance Evey auto-hébergée en http: sur son LAN reste
// jouable : le player CAF a le droit de charger du média http: depuis une page
// https: (doc Google Cast), mais NOS fetch JS (master wasm, segments ac3go)
// sont du mixed content actif et seraient bloqués - donc jamais de wasmAudio
// vers un serveur http:.
export function rewriteCastMasterUrl(
  rawUrl: string,
  caps: ReceiverCaps,
  wasmCapable: boolean,
  pageProtocol = "https:",
): RewrittenLoad | null {
  let u: URL;
  try {
    u = new URL(rawUrl);
  } catch {
    return null;
  }
  if (!u.pathname.endsWith("/hls/master.m3u8")) return null;

  u.searchParams.set("videoCodecs", caps.videoCodecs.join(","));
  u.searchParams.set("containers", "mp4");
  u.searchParams.set("maxHeight", String(caps.maxHeight));
  if (caps.hdr) u.searchParams.set("hdr", "1");
  else u.searchParams.delete("hdr");

  const mixedContent = pageProtocol === "https:" && u.protocol === "http:";
  const wasmAudio = !caps.nativeDolby && wasmCapable && !mixedContent;
  const audio = caps.nativeDolby ? [...BASE_AUDIO, "eac3", "ac3"] : BASE_AUDIO;
  u.searchParams.set("audioCodecs", audio.join(","));
  if (wasmAudio) u.searchParams.set("wasmAudio", "1");
  else u.searchParams.delete("wasmAudio");

  return { url: u.toString(), wasmAudio };
}

// absoluteWithToken résout une URL de playlist/segment contre le SERVEUR MÉDIA
// (l'URL de base absolue) et y injecte le token le plus frais. Jamais contre
// l'origine de la page : le receiver peut être hébergé ailleurs que l'instance
// (GitHub Pages, domaine projet) - l'injectToken du web (même-origine) rend un
// chemin relatif qui partirait vers l'hôte du receiver et ferait 404.
export function absoluteWithToken(url: string, baseUrl: string, token: string): string {
  try {
    const abs = new URL(url, baseUrl);
    if (token) abs.searchParams.set("token", token);
    return abs.toString();
  } catch {
    return url;
  }
}

// stripWasmAudio est le repli quand ac3go ne démarre pas sur la TV : recharger
// le MÊME master sans wasmAudio fait re-servir l'audio AAC muxé par le serveur.
export function stripWasmAudio(rawUrl: string): string {
  try {
    const u = new URL(rawUrl);
    u.searchParams.delete("wasmAudio");
    return u.toString();
  } catch {
    return rawUrl;
  }
}
