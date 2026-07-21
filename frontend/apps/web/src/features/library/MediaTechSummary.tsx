import { useMemo } from "react";
import type { V1MediaItem } from "@evey/api";
import { usePlaybackServiceGetStreamInfo } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { PlaybackDifficultyBadge } from "./PlaybackDifficultyBadge";
import { getBrowserCapabilities, canWasmDecodeAC3 } from "@/utils/codec";
import { formatFileSize, hasMediaFile } from "./mediaDetailUtils";
import {
  classifyPlayback,
  channelLabel,
  hdrLabel,
  resolutionLabel,
  videoStream,
} from "./playbackDifficulty";

// MediaDifficultyBadge asks the server what IT would do with this ONE file for
// THIS browser. List views must not use it - they have the bulk hook
// (usePlaybackDifficulties), which answers for a whole page in one request.
function MediaDifficultyBadge({ item }: { item: V1MediaItem }) {
  const caps = useMemo(() => getBrowserCapabilities(), []);
  // No maxWidth/maxHeight, deliberately: screen size is not a decode
  // capability, and sending it makes the server flag resolution_exceeded and
  // report a transcode for a title the device plays natively. Same reasoning as
  // usePlaySession - keep the call sites in agreement or the badge will
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
  return <PlaybackDifficultyBadge difficulty={d} />;
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
      <MediaDifficultyBadge item={item} />
      {resolution && <Badge variant="default">{resolution}</Badge>}
      {hdr && <Badge variant="default">{hdr}</Badge>}
      {audioChannels && <Badge variant="default">{audioChannels}</Badge>}
      {Number(item.fileSize ?? 0) > 0 && <Badge variant="default">{formatFileSize(item.fileSize)}</Badge>}
    </div>
  );
}
