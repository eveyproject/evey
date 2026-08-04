import { useMemo, useState } from "react";
import { Music } from "lucide-react";
import { useAuthStore } from "@evey/store";

export function TrackArt({ trackID, coverArtURL, size = 36 }: { trackID: string; coverArtURL?: string; size?: number }) {
  const [error, setError] = useState(false);
  // Pin token at trackID-change time: subscribing would reload the <img>
  // on every token refresh (15 min), cache-invalidating every visible cover.
  const src = useMemo(() => {
    const tok = useAuthStore.getState().accessToken;
    const tp = tok ? `?token=${encodeURIComponent(tok)}` : "";
    return `/api/v1/music/tracks/${trackID}/art${tp}`;
  }, [trackID]);
  if (error || !coverArtURL) {
    return (
      <div
        className="flex items-center justify-center bg-[var(--surface)] rounded-[var(--radius-sm)]"
        style={{
          width: size,
          height: size,
          minWidth: size,
        }}
      >
        <Music size={size * 0.5} className="text-[var(--text-muted)] opacity-50" />
      </div>
    );
  }
  return (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      width={size}
      height={size}
      className="object-cover rounded-[var(--radius-sm)] bg-[var(--surface)]"
      style={{
        width: size,
        height: size,
        minWidth: size,
      }}
      onError={() => setError(true)}
    />
  );
}
