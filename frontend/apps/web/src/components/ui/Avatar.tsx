interface AvatarProps {
  /** Single-letter fallback shown when there's no image (already uppercased by the caller if desired). */
  initial: string;
  /** Optional avatar image; when absent the initial is shown on the accent fill. */
  imageUrl?: string | null;
  /** Diameter in px. */
  size?: number;
  className?: string;
}

// Avatar is the accent-filled initials circle used for profiles and users. It
// centralises the on-accent contrast (text-[var(--filter-active-text)]) and the
// image-or-initial fallback that was hand-rolled in the dashboards and the
// profile switcher.
export function Avatar({ initial, imageUrl, size = 32, className = "" }: AvatarProps) {
  return (
    <span
      className={`rounded-full shrink-0 overflow-hidden flex items-center justify-center font-bold bg-[var(--accent)] text-[var(--filter-active-text)] ${className}`}
      style={{ width: size, height: size, fontSize: size * 0.42 }}
    >
      {imageUrl ? <img src={imageUrl} alt="" className="w-full h-full object-cover" /> : initial}
    </span>
  );
}
