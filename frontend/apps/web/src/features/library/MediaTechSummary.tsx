import type { V1MediaItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { formatBytes } from "@evey/api";
import { channelLabel, hdrLabel, resolutionLabel, videoStream } from "./streamLabels";

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
      {resolution && <Badge variant="default">{resolution}</Badge>}
      {hdr && <Badge variant="default">{hdr}</Badge>}
      {audioChannels && <Badge variant="default">{audioChannels}</Badge>}
      {Number(item.fileSize ?? 0) > 0 && <Badge variant="default">{formatBytes(item.fileSize)}</Badge>}
    </div>
  );
}
