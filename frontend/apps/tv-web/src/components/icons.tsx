import { IONICONS, type IoniconName } from "./ionicons";

/** An Ionicons glyph, the set the Android television draws; `size` in dp. */
export function Icon({ name, size = 24, className }: { name: IoniconName; size?: number; className?: string }) {
  const s = `${size / 10}rem`;
  return (
    <svg viewBox="0 0 512 512" width={s} height={s} className={className} aria-hidden="true" focusable="false">
      <path d={IONICONS[name]} fill="currentColor" />
    </svg>
  );
}
