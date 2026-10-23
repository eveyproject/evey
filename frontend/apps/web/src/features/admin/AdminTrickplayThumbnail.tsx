import { useState } from "react";
import { useAuthStore } from "@evey/store";
import { useTrickplayCues, tileForTime, type TrickplayCue } from "@/features/playback/trickplayCues";

const SNAPSHOT_TYPES = new Set([
  "movie", "episode",
  "MEDIA_ITEM_TYPE_MOVIE", "MEDIA_ITEM_TYPE_EPISODE",
]);

interface Props {
  mediaId: string;
  positionMs: number;
  mediaType?: string;
  fullWidth?: boolean;
}

// A responsive crop of one trickplay sprite tile: the whole sprite is scaled so
// the tile fills the box (100% width, tile aspect), positioned to the tile rect.
// Needs the sprite's natural width, read once on load; hidden until then to
// avoid a wrong-crop flash. The frame is 16:9 whatever the tile's shape: a
// scope film's tile was shorter and misaligned the cards next to it.
function TrickplayTile({ cue, fullWidth }: { cue: TrickplayCue; fullWidth?: boolean }) {
  const [natW, setNatW] = useState(0);
  const wide = cue.w / cue.h >= 16 / 9;
  return (
    <div
      className={fullWidth ? "w-full flex items-center justify-center bg-black" : "shrink-0 flex items-center justify-center bg-black rounded-[var(--radius)] overflow-hidden"}
      style={{ aspectRatio: "16/9", width: fullWidth ? undefined : 160 }}
    >
      <div
        data-testid="trickplay-tile"
        className="overflow-hidden relative"
        style={{
          aspectRatio: `${cue.w}/${cue.h}`,
          width: wide ? "100%" : undefined,
          height: wide ? undefined : "100%",
          backgroundColor: "var(--surface-raised)",
        }}
      >
        <img
          src={cue.url}
          alt=""
          role="presentation"
          onLoad={(e) => setNatW(e.currentTarget.naturalWidth)}
          style={
            natW
              ? {
                  position: "absolute",
                  maxWidth: "none",
                  width: `${(natW / cue.w) * 100}%`,
                  left: `${(-cue.x / cue.w) * 100}%`,
                  top: `${(-cue.y / cue.h) * 100}%`,
                }
              : { opacity: 0 }
          }
        />
      </div>
    </div>
  );
}

// AdminTrickplayThumbnail shows a frame at positionMs. It PREFERS the pre-baked
// trickplay sprite tile (a plain image crop, no server work) and only falls back
// to the on-demand /snapshot endpoint (mkvgo keyframe seek + a single ffmpeg
// frame decode) when the media has no trickplay yet - so browsing the admin
// dashboard doesn't spawn an ffmpeg per thumbnail when a sprite already exists.
export function AdminTrickplayThumbnail({ mediaId, positionMs, mediaType, fullWidth }: Props) {
  const token = useAuthStore((s) => s.accessToken);
  // The URL that failed, not a flag: a flag hid the frame for good after one
  // snapshot overran its budget, until a page reload.
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const cues = useTrickplayCues(mediaId, mediaType);

  if (!mediaId) return null;
  if (mediaType && !SNAPSHOT_TYPES.has(mediaType)) return null;

  const tile = tileForTime(cues, positionMs);
  if (tile) return <TrickplayTile cue={tile} fullWidth={fullWidth} />;

  // Fallback: on-demand snapshot (only reached when there is no trickplay).
  const tokenParam = token ? `&token=${encodeURIComponent(token)}` : "";
  // Quantised, because this URL is a WORK ORDER, not a lookup: the snapshot
  // endpoint is an mkvgo keyframe seek plus a full ffmpeg decode. positionMs
  // comes from the 3s session poll, so an unrounded value produced a brand new
  // URL - and a brand new decode - every three seconds, per session, for as
  // long as an admin left the dashboard open. Two or three trickplay-less
  // sessions and the NAS this project targets is spawning ffmpeg around the
  // clock. Rounding makes consecutive polls land on the same cacheable URL;
  // 15s is well under the shot length anyone reads a thumbnail for.
  const SNAPSHOT_QUANTUM_MS = 15_000;
  const snapshotAt = Math.floor(positionMs / SNAPSHOT_QUANTUM_MS) * SNAPSHOT_QUANTUM_MS;
  const src = `/api/v1/media/${mediaId}/snapshot?position_ms=${snapshotAt}${tokenParam}`;
  if (failedSrc === src) return null;

  if (fullWidth) {
    return (
      <img
        src={src}
        alt=""
        role="presentation"
        className="w-full object-cover"
        style={{ aspectRatio: "16/9", backgroundColor: "var(--surface-raised)" }}
        onError={() => setFailedSrc(src)}
      />
    );
  }

  return (
    <img
      src={src}
      alt=""
      role="presentation"
      className="rounded-[var(--radius)] shrink-0 object-cover"
      style={{ width: 160, aspectRatio: "16/9", backgroundColor: "var(--surface-raised)" }}
      onError={() => setFailedSrc(src)}
    />
  );
}
