import { useMemo } from "react";
import type { V1MediaItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { PlaybackDifficultyBadge } from "./PlaybackDifficultyBadge";
import { usePlaybackDifficulties } from "@/hooks/usePlaybackDifficulties";
import { formatFileSize, hasMediaFile } from "./mediaDetailUtils";
import {
  channelLabel,
  hdrLabel,
  resolutionLabel,
  videoStream,
} from "./playbackDifficulty";

// MediaDifficultyBadge goes through the SAME bulk endpoint the lists use, with
// a single id, rather than through GetStreamInfo. Two reasons:
//
//  - One rule set. GetStreamInfo returns no source audio codec, so this had to
//    re-pick the played audio track client-side - and it could not match the
//    server, which skips commentary tracks (the proto carries no commentary
//    flag). A default commentary track would have flipped the AC-3 wasm
//    exemption and mis-graded the title.
//  - No side effect. GetStreamInfo runs keyframeOnDemand.TryIndex, a disk probe
//    meant for a file about to play. Firing it just to draw a badge made every
//    visit to a media page pay for an indexing attempt.
function MediaDifficultyBadge({ item }: { item: V1MediaItem }) {
  const ids = useMemo(
    () => (item.id && hasMediaFile(item) ? [item.id] : []),
    [item.id, item],
  );
  const difficulties = usePlaybackDifficulties(ids);
  const d = item.id ? difficulties.get(item.id) : undefined;
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
