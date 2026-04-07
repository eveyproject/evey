import { useState } from "react";
import { useAuthStore } from "@evey/store";

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

export function AdminTrickplayThumbnail({ mediaId, positionMs, mediaType, fullWidth }: Props) {
  const token = useAuthStore((s) => s.accessToken);
  const [error, setError] = useState(false);

  if (!mediaId || error) return null;
  if (mediaType && !SNAPSHOT_TYPES.has(mediaType)) return null;

  const tokenParam = token ? `&token=${encodeURIComponent(token)}` : "";
  const src = `/api/v1/media/${mediaId}/snapshot?position_ms=${positionMs}${tokenParam}`;

  if (fullWidth) {
    return (
      <img
        src={src}
        alt=""
        className="w-full object-cover"
        style={{ aspectRatio: "16/9", backgroundColor: "var(--surface-raised)" }}
        onError={() => setError(true)}
      />
    );
  }

  return (
    <img
      src={src}
      alt=""
      className="rounded-[var(--radius)] shrink-0 object-cover"
      style={{ width: 160, aspectRatio: "16/9", backgroundColor: "var(--surface-raised)" }}
      onError={() => setError(true)}
    />
  );
}
