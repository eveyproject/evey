import { useId } from "react";

// The web client's wordmark and mark (apps/web EveyLogo), sized in dp.

const RED = "#DC2626";

/** Full "EVEY" wordmark - letters inherit currentColor, the V is brand red. */
export function EveyWordmark({ height = 26, className }: { height?: number; className?: string }) {
  const cut = `cut${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  return (
    <svg
      viewBox="-60 -60 3420 808"
      height={`${height / 10}rem`}
      width={`${(height * 3420) / 808 / 10}rem`}
      role="img"
      aria-label="Evey"
      className={className}
    >
      <defs>
        <mask id={cut}>
          <rect width="100%" height="100%" fill="white" />
          <rect x="0" y="304" width="3300" height="80" fill="black" />
        </mask>
      </defs>
      <g mask={`url(#${cut})`}>
        <path d="M74 688H669V523H295V428H615V270H295V165H676V0H74Z" fill="currentColor" transform="translate(0, 688) scale(1, -1)" />
        <path d="M261 0 17 688H253L391 228H395L534 688H760L517 0Z" fill={RED} transform="translate(822, 688) scale(1, -1)" />
        <path d="M74 688H669V523H295V428H615V270H295V165H676V0H74Z" fill="currentColor" transform="translate(1700, 688) scale(1, -1)" />
        <path d="M501 0H280V267L7 688H261L393 459H397L529 688H770L501 267Z" fill="currentColor" transform="translate(2522, 688) scale(1, -1)" />
      </g>
    </svg>
  );
}

/** Compact mark - the brand red V with the signature horizontal bar cut. */
export function EveyMark({ height = 28, className }: { height?: number; className?: string }) {
  const cut = `cut${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  return (
    <svg viewBox="0 0 512 512" height={`${height / 10}rem`} width={`${height / 10}rem`} role="img" aria-label="Evey" className={className}>
      <defs>
        <mask id={cut}>
          <rect width="512" height="512" fill="white" />
          <rect x="71.68" y="245.76" width="368.64" height="20.48" fill="black" />
        </mask>
      </defs>
      <g mask={`url(#${cut})`}>
        <g transform="translate(70.72744186046509, 419.84) scale(0.4762790697674419, -0.4762790697674419)">
          <path d="M261 0 17 688H253L391 228H395L534 688H760L517 0Z" fill={RED} />
        </g>
      </g>
    </svg>
  );
}
