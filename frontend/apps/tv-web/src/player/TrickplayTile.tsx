import { memo, useEffect } from "react";
import { getBaseURL } from "@evey/api";
import { cueForTime, useTrickplayCues } from "@evey/client";

// The picture at the seek target while the remote scrubs: the sprite sheet
// the server generated, cropped to the tile with background-position, the
// way the browser player does it. Nothing to draw resolves to nothing. The
// manifest and its bounded cache live in @evey/client, shared with the
// Android television.

interface Props {
  mediaId: string;
  previewTimeSec: number | null;
  // Fetch the manifest while the controls show so the first seek has a
  // picture too.
  warm: boolean;
}

function TrickplayTileImpl({ mediaId, previewTimeSec, warm }: Props) {
  const cues = useTrickplayCues(mediaId, warm || previewTimeSec !== null);

  // Warm the first sheet into the browser cache too.
  useEffect(() => {
    if (!warm || cues.length === 0) return;
    const img = new Image();
    img.src = `${getBaseURL()}${cues[0].url}`;
  }, [warm, cues]);

  if (previewTimeSec === null) return null;
  const cue = cueForTime(cues, previewTimeSec);
  if (!cue) return null;
  return (
    <div
      className="trickplay"
      style={{
        width: cue.w,
        height: cue.h,
        backgroundImage: `url("${getBaseURL()}${cue.url}")`,
        backgroundPosition: `-${cue.x}px -${cue.y}px`,
      }}
    />
  );
}

export const TrickplayTile = memo(TrickplayTileImpl);
