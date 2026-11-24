import {
  describeSession,
  formatChannelLayout,
  pickAudioStream,
  playMethodI18nKey,
  visibleTranscodeReasons,
  type ServedView,
  type V1MediaStream,
  type V1PlayedState,
  type V1ServedLeg,
  type V1StreamInfo,
} from "@evey/api";
import type { ServedVariant } from "./servedVariant";

// What the stream-info panel shows, decided once for the four players. The
// players only draw this.
//
// What it says the session IS comes from its two witnesses only (@evey/api
// describeSession): what the server sent (the progress tick's answer) and
// what this player decodes. The stream verdict is not evidence of either: it
// still supplies the source file's own facts (codec, profile, language...)
// and, labelled as such, why the server would not hand the file over whole.

export interface StreamInfoRow {
  // i18n key and English default of the label.
  labelKey: string;
  label: string;
  // Either a literal value, or an i18n key the renderer translates (with
  // valueParams for its placeholders).
  value?: string;
  valueKey?: string;
  valueParams?: Record<string, string | number>;
}

export interface StreamInfoSection {
  kind: "playback" | "video" | "audio";
  titleKey: string;
  title: string;
  // "copy" | "transcode" | "wasm" | "direct", drawn as a badge next to the title.
  decision?: string;
  rows: StreamInfoRow[];
}

export interface StreamInfoModel {
  sections: StreamInfoSection[];
  reasonsHeadingKey: string;
  // Raw snake_case reasons; translate as player.streamInfo.reason.<reason>.
  reasons: string[];
}

export interface StreamInfoOptions {
  // The track the viewer picked: absolute media-stream index, or the ordinal
  // among audio streams. Only used to describe the source track while neither
  // witness has named one.
  audio?: { absolute?: number | null; relative?: number | null };
  // The variant hls.js is playing right now (servedVariant): the rung the
  // player decodes, followed through every quality change, auto included.
  served?: ServedVariant | null;
  // The session's two witnesses: what the server sent (servedVideo,
  // servedAudio, the progress tick's answer) and what this player decodes.
  witness?: { servedVideo?: V1ServedLeg; servedAudio?: V1ServedLeg; played?: V1PlayedState } | null;
}

export function formatBitrate(bitrate?: string | number): string {
  if (bitrate === undefined || bitrate === null || bitrate === "") return "-";
  const bps = Number(bitrate);
  if (!Number.isFinite(bps) || bps <= 0) return "-";
  if (bps >= 1_000_000) return `${(bps / 1_000_000).toFixed(1)} Mbps`;
  if (bps >= 1_000) return `${Math.round(bps / 1_000)} Kbps`;
  return `${bps} bps`;
}

// 23.976023976023978 is a true value and a useless one to read: 23.976, 24, 25, 30.
export function formatFrameRate(fps?: number): string | null {
  if (!fps || !Number.isFinite(fps) || fps <= 0) return null;
  const rounded = Math.round(fps * 1000) / 1000;
  return `${Number.isInteger(rounded) ? rounded : rounded.toFixed(3)} fps`;
}

// Per-stream bitrate is often absent from the probe (the container only carries
// a total). Video ≈ total - audio beats a bare "-", and says it is an estimate.
function deriveVideoBitrate(total?: string, audio?: string): string | null {
  const t = Number(total);
  if (!Number.isFinite(t) || t <= 0) return null;
  const a = Number(audio);
  return formatBitrate(Number.isFinite(a) && a > 0 && a < t ? t - a : t);
}

// A row with nothing to say is dropped: a bare "-" (a "0" bitrate the server
// sends for an unknown figure) reads as a broken panel.
function row(labelKey: string, label: string, value: string | null | undefined): StreamInfoRow | null {
  return value && value !== "-" ? { labelKey, label, value } : null;
}

function legDecision(l?: ServedView): string | undefined {
  if (!l) return undefined;
  if (l.engine === "file") return "direct";
  return l.copy ? "copy" : "transcode";
}

const BACKENDS: Record<string, string> = { software: "CPU", qsv: "Quick Sync", vaapi: "VAAPI", nvenc: "NVENC", rkmpp: "RKMPP" };

function engineLabel(l?: ServedView): string | null {
  if (!l || l.engine === "file") return null;
  return [l.engine, l.backend ? (BACKENDS[l.backend] ?? l.backend) : ""].filter(Boolean).join(" ");
}

function rows(list: (StreamInfoRow | null)[]): StreamInfoRow[] {
  return list.filter((r): r is StreamInfoRow => r !== null);
}

export function buildStreamInfoModel(streamInfo: V1StreamInfo | null | undefined, opts: StreamInfoOptions = {}): StreamInfoModel | null {
  if (!streamInfo) return null;
  const reality = describeSession(opts.witness ?? {});
  const rv = reality.video;
  const ra = reality.audio;
  const played = reality.played;
  const variant = opts.served ?? null;

  const video = streamInfo.mediaStreams?.find((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  // The source track the audio section describes: the one the server sent,
  // else the one the player says it plays, else the viewer's pick.
  const knownTrack = ra && ra.track >= 0 ? ra.track : played && played.audioTrack >= 0 ? played.audioTrack : null;
  // The file itself went out with every track and the player cannot say which
  // it plays (a browser without an audio track list plays the file's own
  // default): no track is described as the one heard.
  const unnamedFileTrack = !!ra && ra.engine === "file" && knownTrack === null;
  const audio = unnamedFileTrack ? undefined : pickAudioStream(streamInfo.mediaStreams, knownTrack !== null ? { relative: knownTrack } : (opts.audio ?? {}));

  const sections: StreamInfoSection[] = [];
  const methodKey = playMethodI18nKey(reality.method);
  sections.push({
    kind: "playback",
    titleKey: "player.streamInfo.playback",
    title: "Playback",
    rows: rows([
      { labelKey: "player.streamInfo.method", label: "Method", valueKey: methodKey || "player.streamInfo.waiting" },
      // Source, not served: anything but direct play repackages to fMP4.
      row("player.streamInfo.containerSource", "Source container", streamInfo.container?.toUpperCase()),
      row("player.streamInfo.totalBitrate", "Total Bitrate", formatBitrate(streamInfo.bitrate)),
    ]),
  });

  if (video && rv && !rv.copy) {
    // An encoded picture: its own shape, not the source's (whose profile and
    // pixel format it no longer has).
    const codec = rv.sourceCodec ? `${rv.sourceCodec}→${rv.codec}` : rv.codec || video.codec || "-";
    sections.push({
      kind: "video",
      titleKey: "player.streamInfo.video",
      title: "Video",
      decision: legDecision(rv),
      rows: rows([
        row("player.streamInfo.codec", "Codec", codec.toUpperCase()),
        row("player.streamInfo.resolution", "Resolution",
          variant?.width && variant?.height ? `${variant.width}×${variant.height}` : variant?.height ? `${variant.height}p` : rv.height ? `${rv.height}p` : null),
        row("player.streamInfo.bitrate", "Bitrate", variant ? formatBitrate(variant.bitrate) : null),
        row("player.streamInfo.framerate", "Frame Rate", formatFrameRate(variant?.frameRate ?? video.frameRate)),
        row("player.streamInfo.engine", "Engine", engineLabel(rv)),
      ]),
    });
  } else if (video) {
    const explicit = video.bitrate ? formatBitrate(video.bitrate) : "-";
    const derived = explicit === "-" ? deriveVideoBitrate(streamInfo.bitrate, audio?.bitrate) : null;
    sections.push({
      kind: "video",
      titleKey: "player.streamInfo.video",
      title: "Video",
      decision: legDecision(rv),
      rows: rows([
        row("player.streamInfo.codec", "Codec", (video.codec ?? "-").toUpperCase()),
        row("player.streamInfo.resolution", "Resolution", video.width && video.height ? `${video.width}×${video.height}` : null),
        row("player.streamInfo.bitrate", "Bitrate", explicit !== "-" ? explicit : derived ? `≈ ${derived}` : null),
        row("player.streamInfo.framerate", "Frame Rate", formatFrameRate(video.frameRate)),
        row("player.streamInfo.profile", "Profile", video.profile),
        row("player.streamInfo.pixelFormat", "Pixel Format", video.pixelFormat),
        row("player.streamInfo.engine", "Engine", engineLabel(rv)),
      ]),
    });
  }

  if (audio || ra) {
    const sourceCodec = audio?.codec ?? "";
    const codec = ra && ra.codec ? (ra.sourceCodec ? `${ra.sourceCodec}→${ra.codec}` : ra.codec) : sourceCodec || "-";
    const trackRow: StreamInfoRow | null =
      ra && ra.track < 0 && ra.engine === "file"
        ? { labelKey: "player.streamInfo.track", label: "Track", valueKey: "player.streamInfo.allTracks" }
        : knownTrack !== null
          ? { labelKey: "player.streamInfo.track", label: "Track", value: String(knownTrack + 1) }
          : null;
    const decoder: StreamInfoRow | null = played?.decoder
      ? { labelKey: "player.streamInfo.player", label: "Player", valueKey: played.decoder === "wasm" ? "player.streamInfo.decoderWasm" : "player.streamInfo.decoderNative" }
      : null;
    const mismatches: StreamInfoRow[] = [];
    if (reality.trackMismatch) {
      mismatches.push({ labelKey: "player.streamInfo.mismatch", label: "Mismatch", valueKey: "player.streamInfo.mismatchTrack",
        valueParams: { played: reality.trackMismatch.played + 1, served: reality.trackMismatch.served + 1 } });
    }
    if (reality.codecMismatch) {
      mismatches.push({ labelKey: "player.streamInfo.mismatch", label: "Mismatch", valueKey: "player.streamInfo.mismatchCodec",
        valueParams: { ...reality.codecMismatch } });
    }
    sections.push({
      kind: "audio",
      titleKey: "player.streamInfo.audio",
      title: "Audio",
      decision: played?.decoder === "wasm" ? "wasm" : legDecision(ra),
      rows: rows([
        trackRow,
        row("player.streamInfo.codec", "Codec", codec.toUpperCase()),
        row("player.streamInfo.channels", "Channels",
          ra?.channels ? formatChannelLayout(ra.channels) : audio?.channels ? formatChannelLayout(audio.channels) : null),
        row("player.streamInfo.language", "Language", audio?.language),
        // The source's own rate: what an encode puts out is not measured here.
        row("player.streamInfo.bitrate", "Bitrate", !ra || ra.copy ? formatBitrate(audio?.bitrate) : null),
        row("player.streamInfo.sampleRate", "Sample Rate", (!ra || ra.copy) && audio?.sampleRate ? `${audio.sampleRate} Hz` : null),
        row("player.streamInfo.engine", "Engine", engineLabel(ra)),
        decoder,
        ...mismatches,
      ]),
    });
  }

  const remuxed = reality.method === "transcode" || reality.method === "direct_stream";
  return {
    sections,
    reasonsHeadingKey: reality.method === "transcode" ? "player.streamInfo.transcodeReasons" : "player.streamInfo.noDirectPlayReasons",
    // Why the server would not hand the file over whole: the verdict's own
    // explanation, shown once the bytes say it was not.
    // A reason the wasm decoder made moot (the codec this browser cannot
    // decode natively, it decodes in wasm) is dropped while it decodes.
    reasons: remuxed ? visibleTranscodeReasons(streamInfo.transcodeReasons, played?.decoder === "wasm") : [],
  };
}

// The badge label of a decision: copied (untouched), transcoded (re-encoded),
// or decoded client-side (served untouched, decoded by WebAssembly).
export function decisionBadgeKey(decision: string): { key: string; defaultValue: string } {
  if (decision === "wasm") return { key: "player.streamInfo.decisionWasm", defaultValue: "Décodé client" };
  if (decision === "direct") return { key: "player.streamInfo.fromFile", defaultValue: "fichier source" };
  if (decision === "copy") return { key: "player.streamInfo.decisionCopy", defaultValue: "Copié" };
  return { key: "player.streamInfo.decisionTranscode", defaultValue: "Transcodé" };
}
