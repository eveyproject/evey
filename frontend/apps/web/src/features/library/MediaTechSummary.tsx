import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import { usePlaybackServiceGetStreamInfo } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { getBrowserCapabilities, canWasmDecodeAC3 } from "@/utils/codec";
import { formatFileSize, hasMediaFile } from "./mediaDetailUtils";
import {
  classifyPlayback,
  channelLabel,
  hdrLabel,
  resolutionLabel,
  videoStream,
  type Difficulty,
} from "./playbackDifficulty";

const DIFFICULTY_VARIANT: Record<Difficulty, "success" | "info" | "warning" | "error"> = {
  easy: "success",
  moderate: "info",
  demanding: "warning",
  hard: "error",
};

// PlaybackDifficultyBadge asks the server what IT would do with this file for
// THIS browser, rather than guessing from codec names. One RPC per media item -
// which is why this only ever renders on a detail page, never per row in a
// season's episode list or in a grid.
function PlaybackDifficultyBadge({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const caps = useMemo(() => getBrowserCapabilities(), []);
  // No maxWidth/maxHeight, deliberately: screen size is not a decode
  // capability, and sending it makes the server flag resolution_exceeded and
  // report a transcode for a title the device plays natively. Same reasoning as
  // usePlaySession - keep the two call sites in agreement or the badge will
  // contradict what actually happens at play time.
  const { data: info } = usePlaybackServiceGetStreamInfo(
    item.id ?? "",
    {
      supportedCodecs: caps.videoCodecs,
      supportedAudioCodecs: caps.audioCodecs,
      supportedContainers: caps.containers,
      supportsHdr: caps.hdr,
    },
    { query: { enabled: !!item.id && hasMediaFile(item), retry: false } } as never,
  );

  const d = classifyPlayback(info, item.streams, canWasmDecodeAC3());
  if (!d) return null;

  // One plain sentence on hover. The server's own reason codes
  // (hdr_tonemapping_needed, container_incompatible...) are diagnostics, not
  // something to put in front of someone deciding what to watch.
  return (
    <Badge
      variant={DIFFICULTY_VARIANT[d.difficulty]}
      className="cursor-help"
      title={t(`media.playback.hint.${d.difficulty}`)}
      data-testid="playback-difficulty"
    >
      {t(`media.playback.level.${d.difficulty}`)}
    </Badge>
  );
}

// MediaTechSummary is the always-visible one-liner, and it is aimed at someone
// choosing what to watch - not at someone debugging a transcode. So it carries
// only what a viewer actually reads: how heavy it is to stream, the picture
// size, whether it is HDR, how many sound channels, how big the file is.
//
// Codec names (HEVC, EAC3) and the container (MATROSKA) are deliberately NOT
// here: they mean nothing to most people and they were the bulk of the noise.
// They stay one click away in the file-info disclosure, which is where someone
// looking for them will go.
export function MediaTechSummary({ item }: { item: V1MediaItem }) {
  const v = videoStream(item.streams);
  const audio = item.streams?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO") ?? [];
  const primaryAudio = audio.find((s) => s.isDefault) ?? audio[0];

  const resolution = resolutionLabel(v);
  const hdr = hdrLabel(v);
  const audioChannels = channelLabel(primaryAudio?.channels);

  return (
    <div className="flex flex-wrap items-center gap-2" data-testid="media-tech-summary">
      <PlaybackDifficultyBadge item={item} />
      {resolution && <Badge variant="default">{resolution}</Badge>}
      {hdr && <Badge variant="default">{hdr}</Badge>}
      {audioChannels && <Badge variant="default">{audioChannels}</Badge>}
      {Number(item.fileSize ?? 0) > 0 && <Badge variant="default">{formatFileSize(item.fileSize)}</Badge>}
    </div>
  );
}
