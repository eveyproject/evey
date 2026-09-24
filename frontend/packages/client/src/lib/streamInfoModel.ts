import {
  effectivePlayMethod,
  formatChannelLayout,
  pickAudioStream,
  playMethodI18nKey,
  reasonsHeadingI18nKey,
  visibleTranscodeReasons,
  type V1MediaStream,
  type V1StreamInfo,
} from "@evey/api";

// What the stream-info panel shows, decided once for the four players. Each
// kept its own list until 2026-09-24 and they had drifted: the television and
// the browser television showed neither the estimated video bitrate, the
// profile, the pixel format, the served audio bitrate nor the sample rate,
// and wrote bitrates in two different units. The players only draw this.

export interface StreamInfoRow {
  // i18n key and English default of the label.
  labelKey: string;
  label: string;
  // Either a literal value, or an i18n key the renderer translates.
  value?: string;
  valueKey?: string;
}

export interface StreamInfoSection {
  kind: "playback" | "video" | "audio";
  titleKey: string;
  title: string;
  // "copy" | "transcode" | "wasm", drawn as a badge next to the title.
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
  // among audio streams. Without it the panel described the file's FIRST
  // audio track, whatever was playing.
  audio?: { absolute?: number | null; relative?: number | null };
  // The audio is decoded client-side (ac3go WebAssembly), not transcoded.
  wasmAudioActive?: boolean;
  // Which leg of the browser's bi-source relay feeds the speakers: on "aac" or
  // "aac-latched" the sound is the server's AAC rendition, a transcode.
  wasmAudioLeg?: string | null;
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

function row(labelKey: string, label: string, value: string | null | undefined): StreamInfoRow | null {
  return value ? { labelKey, label, value } : null;
}

export function buildStreamInfoModel(streamInfo: V1StreamInfo | null | undefined, opts: StreamInfoOptions = {}): StreamInfoModel | null {
  if (!streamInfo) return null;
  const onNativeLeg = opts.wasmAudioLeg === "aac" || opts.wasmAudioLeg === "aac-latched";
  const wasmDecoding = (opts.wasmAudioActive ?? false) && !onNativeLeg;
  const audioDecision = onNativeLeg ? "transcode" : wasmDecoding ? "wasm" : streamInfo.audioDecision;
  const decisions = {
    playMethod: streamInfo.playMethod,
    videoDecision: streamInfo.videoDecision,
    audioDecision,
    wasmAudio: wasmDecoding,
  };

  const video = streamInfo.mediaStreams?.find((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const audio = pickAudioStream(streamInfo.mediaStreams, opts.audio ?? {});

  const sections: StreamInfoSection[] = [];
  const methodKey = playMethodI18nKey(effectivePlayMethod(decisions));
  sections.push({
    kind: "playback",
    titleKey: "player.streamInfo.playback",
    title: "Playback",
    rows: [
      methodKey ? { labelKey: "player.streamInfo.method", label: "Method", valueKey: methodKey } : null,
      // Source, not served: anything but direct play repackages to fMP4.
      row("player.streamInfo.containerSource", "Source container", streamInfo.container?.toUpperCase()),
      row("player.streamInfo.totalBitrate", "Total Bitrate", streamInfo.bitrate ? formatBitrate(streamInfo.bitrate) : null),
    ].filter((r): r is StreamInfoRow => r !== null),
  });

  if (video) {
    const explicit = video.bitrate ? formatBitrate(video.bitrate) : "-";
    const derived = explicit === "-" ? deriveVideoBitrate(streamInfo.bitrate, audio?.bitrate) : null;
    sections.push({
      kind: "video",
      titleKey: "player.streamInfo.video",
      title: "Video",
      decision: streamInfo.videoDecision,
      rows: [
        row("player.streamInfo.codec", "Codec", (video.codec ?? "-").toUpperCase()),
        row("player.streamInfo.resolution", "Resolution", video.width && video.height ? `${video.width}×${video.height}` : null),
        row("player.streamInfo.bitrate", "Bitrate", explicit !== "-" ? explicit : derived ? `≈ ${derived}` : null),
        row("player.streamInfo.framerate", "Frame Rate", formatFrameRate(video.frameRate)),
        row("player.streamInfo.profile", "Profile", video.profile),
        row("player.streamInfo.pixelFormat", "Pixel Format", video.pixelFormat),
      ].filter((r): r is StreamInfoRow => r !== null),
    });
  }

  if (audio) {
    // The SERVED rate, not the source's: a Matroska track rarely declares one,
    // so a re-encoded track sat at "-" though the server picked the figure.
    const audioBitrate = streamInfo.audioBitrate ?? audio.bitrate;
    sections.push({
      kind: "audio",
      titleKey: "player.streamInfo.audio",
      title: "Audio",
      decision: audioDecision,
      rows: [
        row("player.streamInfo.codec", "Codec", (audio.codec ?? "-").toUpperCase()),
        row("player.streamInfo.channels", "Channels", audio.channels ? formatChannelLayout(audio.channels) : null),
        row("player.streamInfo.language", "Language", audio.language),
        row("player.streamInfo.bitrate", "Bitrate", audioBitrate ? formatBitrate(audioBitrate) : null),
        row("player.streamInfo.sampleRate", "Sample Rate", audio.sampleRate ? `${audio.sampleRate} Hz` : null),
      ].filter((r): r is StreamInfoRow => r !== null),
    });
  }

  return {
    sections,
    reasonsHeadingKey: reasonsHeadingI18nKey(decisions),
    // On the native leg the audio really IS re-encoded, so the reason the wasm
    // path hides comes back.
    reasons: visibleTranscodeReasons(streamInfo.transcodeReasons, wasmDecoding),
  };
}

// The badge label of a decision: copied (untouched), transcoded (re-encoded),
// or decoded client-side (served untouched, decoded by WebAssembly).
export function decisionBadgeKey(decision: string): { key: string; defaultValue: string } {
  if (decision === "wasm") return { key: "player.streamInfo.decisionWasm", defaultValue: "Décodé client" };
  if (decision === "copy" || decision === "direct") return { key: "player.streamInfo.decisionCopy", defaultValue: "Copié" };
  return { key: "player.streamInfo.decisionTranscode", defaultValue: "Transcodé" };
}
