import type { PlayerAudioStream } from "@evey/api";

// A native player (Media3, AVPlayer) names an HLS audio rendition by the NAME
// of its EXT-X-MEDIA tag (Media3's track id is "GROUP-ID:NAME") and exposes
// neither its URI nor its other attributes. Which source track a rendition
// carries is written in the master the player loaded: the packager's
// X-MKVGO-TRACK (the number the file gives the track), Evey's ffmpeg
// renditions' audio_N.m3u8. Reading that same master is how a native player
// knows which track it plays, and picks the one asked for, across every
// audio group - the same facts the web reads off hls.js.

export interface MasterAudioRendition {
  groupId: string;
  name: string;
  // The file's track number (X-MKVGO-TRACK), when the packager wrote it.
  fileTrack?: number;
  // The source audio ordinal of an Evey ffmpeg rendition (audio_N.m3u8).
  audioOrdinal?: number;
  // The audio codec the master declares for the rendition's group.
  codec?: string;
}

function attrs(line: string): Record<string, string> {
  const out: Record<string, string> = {};
  const re = /([A-Z0-9-]+)=("([^"]*)"|[^,]*)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(line)) !== null) out[m[1]] = m[3] !== undefined ? m[3] : m[2];
  return out;
}

// muxedAudioCodec is the audio codec a master without audio renditions
// declares for its variants (the audio muxed with the video); undefined when
// it declares none.
export function muxedAudioCodec(master: string): string | undefined {
  for (const l of master.split(/\r?\n/)) {
    if (!l.startsWith("#EXT-X-STREAM-INF:")) continue;
    const a = attrs(l.slice("#EXT-X-STREAM-INF:".length));
    if (a.AUDIO || !a.CODECS) continue;
    const audio = a.CODECS.split(",").map((c) => c.trim()).find((c) => !/^(avc|hvc|hev|av01|vp0|dvh|dva)/i.test(c));
    if (audio) return audio;
  }
  return undefined;
}

export function parseMasterAudioRenditions(master: string): MasterAudioRendition[] {
  const lines = master.split(/\r?\n/);
  const groupCodec = new Map<string, string>();
  for (const l of lines) {
    if (!l.startsWith("#EXT-X-STREAM-INF:")) continue;
    const a = attrs(l.slice("#EXT-X-STREAM-INF:".length));
    if (!a.AUDIO || !a.CODECS) continue;
    const audio = a.CODECS.split(",").map((c) => c.trim()).find((c) => !/^(avc|hvc|hev|av01|vp0|dvh|dva)/i.test(c));
    if (audio && !groupCodec.has(a.AUDIO)) groupCodec.set(a.AUDIO, audio);
  }
  const out: MasterAudioRendition[] = [];
  for (const l of lines) {
    if (!l.startsWith("#EXT-X-MEDIA:")) continue;
    const a = attrs(l.slice("#EXT-X-MEDIA:".length));
    if (a.TYPE !== "AUDIO" || !a.NAME) continue;
    const r: MasterAudioRendition = { groupId: a["GROUP-ID"] ?? "", name: a.NAME };
    const ft = Number(a["X-MKVGO-TRACK"]);
    if (Number.isInteger(ft) && ft > 0) r.fileTrack = ft;
    const m = /(?:^|\/)audio_(\d+)\.m3u8/.exec(a.URI ?? "");
    if (m) r.audioOrdinal = Number(m[1]);
    out.push(r);
  }
  // A variant declares ONE audio codec for its whole group: it names a
  // rendition's codec only when the rendition is alone in its group (the
  // packager's master, one group per codec). Evey's ffmpeg master puts every
  // rendition in one group under the first one's codec - an MP3 group codec
  // would have been reported for its AAC track (measured).
  for (const r of out) {
    if (out.filter((o) => o.groupId === r.groupId).length === 1) r.codec = groupCodec.get(r.groupId);
  }
  return out;
}

// renditionOrdinal is the source audio ordinal a rendition carries, -1 when
// the master does not say.
export function renditionOrdinal(r: MasterAudioRendition | undefined, audioStreams: PlayerAudioStream[]): number {
  if (!r) return -1;
  if (r.fileTrack !== undefined) return audioStreams.find((s) => s.sourceTrackId === r.fileTrack)?.trackIndex ?? -1;
  return r.audioOrdinal ?? -1;
}

export interface NativeTrackLike {
  id?: string;
  name?: string;
}

// renditionOfNativeTrack is the rendition a native audio track is: by
// Media3's "GROUP-ID:NAME" id, else by a NAME no other rendition shares.
// undefined when it cannot be told apart.
export function renditionOfNativeTrack(renditions: MasterAudioRendition[], track: NativeTrackLike | null | undefined): MasterAudioRendition | undefined {
  if (!track) return undefined;
  if (track.id) {
    const byId = renditions.find((r) => `${r.groupId}:${r.name}` === track.id);
    if (byId) return byId;
  }
  if (!track.name) return undefined;
  const byName = renditions.filter((r) => r.name === track.name);
  return byName.length === 1 ? byName[0] : undefined;
}

// nativeTrackForOrdinal is the native audio track that carries source audio
// track `ordinal`, whatever group it sits in; undefined when none can be named.
export function nativeTrackForOrdinal<T extends NativeTrackLike>(nativeTracks: T[], renditions: MasterAudioRendition[], ordinal: number, audioStreams: PlayerAudioStream[]): T | undefined {
  if (ordinal < 0) return undefined;
  return nativeTracks.find((t) => renditionOrdinal(renditionOfNativeTrack(renditions, t), audioStreams) === ordinal);
}
