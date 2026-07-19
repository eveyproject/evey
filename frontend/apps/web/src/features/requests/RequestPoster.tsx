import { useState } from "react";
import { Film, Music, Tv } from "lucide-react";

type Props = {
  posterUrl?: string;
  title?: string;
  mediaType?: string;
  className?: string;
};

// A poster URL returned by a metadata search is a BET, not a fact: the
// MusicBrainz provider builds coverartarchive.org/release-group/<mbid>/front-250
// from the id alone without asking whether that release-group has any cover art
// (many have none - it answers 404/500), and a TMDB path can 404 just as well.
// Verifying server-side would cost one rate-limited round trip per result on the
// search path, so the check happens where it is free: the browser already
// fetches the image, and the error tells us. Mirrors CollectionCard's pattern.
export function RequestPoster({ posterUrl, title, mediaType, className }: Props) {
  const [failed, setFailed] = useState(false);
  const Icon = mediaType === "music" ? Music : mediaType === "tv" ? Tv : Film;

  if (!posterUrl || failed) {
    return (
      <div
        className={`w-full h-full flex items-center justify-center ${className ?? ""}`}
        style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}
        data-testid="request-poster-fallback"
      >
        <Icon size={20} aria-hidden="true" />
      </div>
    );
  }

  return (
    <img
      src={posterUrl}
      alt={title ?? ""}
      loading="lazy"
      className={`w-full h-full object-cover ${className ?? ""}`}
      onError={() => setFailed(true)}
    />
  );
}
