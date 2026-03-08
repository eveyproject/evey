import { useState } from "react";
import type { V1MediaItem } from "@evey/api/src/generated/library.schemas";
import {
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { StreamList } from "./StreamList";

interface MediaDetailProps {
  item: V1MediaItem;
}

function formatDuration(ms?: string): string {
  if (!ms) return "";
  const totalMs = parseInt(ms, 10);
  if (isNaN(totalMs)) return "";
  const totalSec = Math.floor(totalMs / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function typeLabel(type?: string): string {
  switch (type) {
    case "MEDIA_ITEM_TYPE_MOVIE": return "Movie";
    case "MEDIA_ITEM_TYPE_SERIES": return "Series";
    case "MEDIA_ITEM_TYPE_EPISODE": return "Episode";
    case "MEDIA_ITEM_TYPE_SEASON": return "Season";
    case "MEDIA_ITEM_TYPE_ALBUM": return "Album";
    case "MEDIA_ITEM_TYPE_ARTIST": return "Artist";
    case "MEDIA_ITEM_TYPE_TRACK": return "Track";
    default: return "";
  }
}

export function MediaDetail({ item }: MediaDetailProps) {
  const [watched, setWatched] = useState(false);

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: { onSuccess: () => setWatched(true) },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: { onSuccess: () => setWatched(false) },
  });

  const backdrop = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_BACKDROP")?.url;
  const poster = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url;
  const duration = formatDuration(item.durationMs);
  const label = typeLabel(item.type);

  return (
    <div>
      {backdrop && (
        <div
          className="relative w-full h-64 overflow-hidden mb-6"
          style={{ borderRadius: "var(--radius-lg)" }}
        >
          <img
            src={backdrop}
            alt=""
            className="w-full h-full object-cover"
          />
          <div
            className="absolute inset-0"
            style={{ background: "linear-gradient(to top, var(--bg) 0%, transparent 60%)" }}
          />
        </div>
      )}

      <div className="flex gap-6">
        {poster && (
          <img
            src={poster}
            alt={item.title ?? ""}
            className="flex-shrink-0 w-36 object-cover"
            style={{ borderRadius: "var(--radius-md)", aspectRatio: "2/3" }}
          />
        )}

        <div className="flex-1 space-y-4">
          <div>
            <div className="flex items-center gap-2 mb-1">
              {label && <Badge>{label}</Badge>}
              {item.officialRating && <Badge variant="default">{item.officialRating}</Badge>}
            </div>
            <h1 className="text-3xl font-bold" style={{ color: "var(--text)" }}>
              {item.title}
            </h1>
            <div className="flex items-center gap-3 mt-1 text-sm" style={{ color: "var(--text-muted)" }}>
              {item.year && <span>{item.year}</span>}
              {duration && <span>{duration}</span>}
              {item.communityRating !== undefined && item.communityRating > 0 && (
                <span>★ {item.communityRating.toFixed(1)}</span>
              )}
            </div>
          </div>

          {item.genres && item.genres.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {item.genres.map((g) => (
                <Badge key={g} variant="default">{g}</Badge>
              ))}
            </div>
          )}

          {item.overview && (
            <p className="text-sm leading-relaxed" style={{ color: "var(--text-muted)" }}>
              {item.overview}
            </p>
          )}

          <div className="flex gap-3">
            {item.id && (
              <a href={`/player/${item.id}`}>
                <Button variant="primary">
                  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
                  </svg>
                  Play
                </Button>
              </a>
            )}
            {item.id && (
              watched ? (
                <Button
                  variant="secondary"
                  loading={markUnwatched.isPending}
                  onClick={() => markUnwatched.mutate({ mediaItemId: item.id! })}
                >
                  Unmark Watched
                </Button>
              ) : (
                <Button
                  variant="secondary"
                  loading={markWatched.isPending}
                  onClick={() => markWatched.mutate({ mediaItemId: item.id! })}
                >
                  Mark as Watched
                </Button>
              )
            )}
          </div>
        </div>
      </div>

      {item.streams && item.streams.length > 0 && (
        <div className="mt-8">
          <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
            Streams
          </h2>
          <StreamList streams={item.streams} />
        </div>
      )}
    </div>
  );
}
