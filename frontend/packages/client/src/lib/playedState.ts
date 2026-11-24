import type { PlayerAudioStream, V1PlayedState } from "@evey/api";

// The player's witness of a session: what it decodes, read from what it
// loaded, never from what it asked for. The server checks it against the
// bytes it sent; the admin card and the stream panel show both.
//
// Which track an HLS audio rendition is comes from the rendition itself: the
// packager publishes the number the file gives the track (X-MKVGO-TRACK),
// Evey's ffmpeg renditions are named after the track (audio_N.m3u8). The
// index the player picked is not evidence: a master that groups renditions
// by codec lists in hls.audioTracks only the current group's.

export interface RenditionLike {
  url?: string;
  attrs?: Record<string, string | undefined>;
}

// renditionTrackOrdinal is the source audio ordinal an HLS audio rendition
// carries, -1 when the rendition does not say.
export function renditionTrackOrdinal(rendition: RenditionLike | undefined, audioStreams: PlayerAudioStream[]): number {
  if (!rendition) return -1;
  const fileTrack = Number(rendition.attrs?.["X-MKVGO-TRACK"]);
  if (Number.isInteger(fileTrack) && fileTrack > 0) {
    const s = audioStreams.find((a) => a.sourceTrackId === fileTrack);
    return s ? s.trackIndex : -1;
  }
  const m = /(?:^|\/)audio_(\d+)\.m3u8/.exec(rendition.url ?? "");
  return m ? Number(m[1]) : -1;
}

// trackRendition is the HLS audio rendition that carries source audio track
// `ordinal`, searched across every audio group of the master (a master that
// groups renditions by codec lists in hls.audioTracks the current group's
// only: the MP3 track was never in the AAC group's list, and picking index 0
// there re-selected the AAC - measured on the demo).
export function trackRendition<T extends RenditionLike>(renditions: T[] | undefined, ordinal: number, audioStreams: PlayerAudioStream[]): T | undefined {
  if (ordinal < 0) return undefined;
  return (renditions ?? []).find((r) => renditionTrackOrdinal(r, audioStreams) === ordinal);
}

// masterTrackOrdinal is the audio track a muxed master was requested with
// (its audioTrack query), -1 when the URL names none.
export function masterTrackOrdinal(masterUrl: string | undefined): number {
  if (!masterUrl) return -1;
  try {
    const v = new URL(masterUrl, "http://x").searchParams.get("audioTrack");
    const n = v === null ? NaN : Number(v);
    return Number.isInteger(n) && n >= 0 ? n : -1;
  } catch {
    return -1;
  }
}

// splitCodecs separates hls.js's codec string for a muxed buffer
// ("avc1.64001f,mp4a.40.2") into its video and audio parts.
export function splitCodecs(codecs: string | undefined): { video?: string; audio?: string } {
  const out: { video?: string; audio?: string } = {};
  for (const c of (codecs ?? "").split(",").map((x) => x.trim()).filter(Boolean)) {
    if (/^(avc|hvc|hev|av01|vp0|dvh|dva)/i.test(c)) out.video ??= c;
    else out.audio ??= c;
  }
  return out;
}

export interface PlayedInputs {
  // Segmented presentation (hls.js) or the source file itself.
  hls: boolean;
  // hls.js: the codecs its buffers were set up with (BUFFER_CODECS), the
  // active audio rendition and the master URL it loaded.
  bufferCodecs?: { video?: string; audio?: string };
  activeRendition?: RenditionLike;
  hasRenditions?: boolean;
  masterUrl?: string;
  // The wasm path is on, on this source track; wasmDecoding: its decoder is
  // what the viewer hears (else the relay's native leg is).
  wasmPath?: boolean;
  wasmDecoding?: boolean;
  wasmTrack?: number;
  // The file: the track the element has enabled (-1 when it cannot say).
  fileTrack?: number;
  height?: number;
  audioStreams: PlayerAudioStream[];
}

export function playedState(i: PlayedInputs): V1PlayedState {
  const height = i.height && i.height > 0 ? i.height : undefined;
  if (!i.hls) {
    return { source: "file", audioTrack: i.fileTrack ?? -1, height };
  }
  if (i.wasmPath) {
    const track = i.wasmTrack ?? -1;
    return {
      source: "hls",
      videoCodec: i.bufferCodecs?.video,
      height,
      audioTrack: track,
      // On the native leg the relay plays the server's rendition, whose codec
      // this player never set a buffer up for: it names none.
      audioCodec: i.wasmDecoding ? i.audioStreams.find((a) => a.trackIndex === track)?.codec : undefined,
      audioDecoder: i.wasmDecoding ? "wasm" : "native",
    };
  }
  const audioTrack = i.hasRenditions ? renditionTrackOrdinal(i.activeRendition, i.audioStreams) : masterTrackOrdinal(i.masterUrl);
  return {
    source: "hls",
    videoCodec: i.bufferCodecs?.video,
    height,
    audioTrack,
    audioCodec: i.bufferCodecs?.audio,
    audioDecoder: i.bufferCodecs?.audio ? "native" : undefined,
  };
}

interface AudioTrackListLike {
  length: number;
  [i: number]: { id: string; enabled: boolean };
}

// fileTrackOrdinal is the source audio ordinal of the track a playing file
// has enabled, by the number the file gives it; -1 when the element exposes
// no track list (Chrome, Firefox: the file plays its default track).
export function fileTrackOrdinal(video: (HTMLVideoElement & { audioTracks?: AudioTrackListLike }) | null, audioStreams: PlayerAudioStream[]): number {
  const tracks = video?.audioTracks;
  if (!tracks) return -1;
  for (let i = 0; i < tracks.length; i++) {
    if (!tracks[i].enabled) continue;
    const id = Number(tracks[i].id);
    const s = audioStreams.find((a) => a.sourceTrackId === id);
    return s ? s.trackIndex : -1;
  }
  return -1;
}
