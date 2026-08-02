import Svg, { G, Path, Rect } from "react-native-svg";

// The brand wordmark and mark, ported from the web component
// (apps/web/src/components/ui/EveyLogo.tsx) so the three clients draw the same
// letters from the same path data.
//
// Kept as vector rather than a PNG for the reason the web version is vector:
// the letters follow the theme text colour while the V stays brand red, and a
// television renders it at whatever size the layout asks for.
//
// ⚠️ The signature band through the letters is a MASK CUT on the web. Not here:
// react-native-svg renders NOTHING at all when a <Mask> is involved on Android
// - no error, no warning, just an empty box, which cost a round of "the logo
// is missing". It is drawn as a band in `cutColor` instead. The surfaces this
// sits on (the TV sidebar, the mobile home header) are a flat colour, so
// painting the band is indistinguishable from cutting it. Over a photo it
// would not be - pass the right colour, or none at all.

const RED = "#DC2626";

interface LogoProps {
  height?: number;
  /** Letter colour. Defaults to white; pass the theme's text colour. */
  color?: string;
  /** Colour of the band through the letters, i.e. the surface behind it. */
  cutColor?: string;
}

/** Full "EVEY" wordmark - letters take `color`, the V is brand red. */
export function EveyWordmark({ height = 26, color = "#FFFFFF", cutColor }: LogoProps) {
  const width = (height * 3420) / 808;
  return (
    <Svg viewBox="-60 -60 3420 808" height={height} width={width} accessibilityLabel="Evey">
      <G>
        <Path d="M74 688H669V523H295V428H615V270H295V165H676V0H74Z" fill={color} transform="translate(0, 688) scale(1, -1)" />
        <Path d="M261 0 17 688H253L391 228H395L534 688H760L517 0Z" fill={RED} transform="translate(822, 688) scale(1, -1)" />
        <Path d="M74 688H669V523H295V428H615V270H295V165H676V0H74Z" fill={color} transform="translate(1700, 688) scale(1, -1)" />
        <Path d="M501 0H280V267L7 688H261L393 459H397L529 688H770L501 267Z" fill={color} transform="translate(2522, 688) scale(1, -1)" />
      </G>
      {cutColor ? <Rect x="-60" y="304" width="3420" height="80" fill={cutColor} /> : null}
    </Svg>
  );
}

/** Compact mark - the brand red V with the signature band. */
export function EveyMark({ height = 28, cutColor }: { height?: number; cutColor?: string }) {
  return (
    <Svg viewBox="0 0 512 512" height={height} width={height} accessibilityLabel="Evey">
      <G transform="translate(70.72744186046509, 419.84) scale(0.4762790697674419, -0.4762790697674419)">
        <Path d="M261 0 17 688H253L391 228H395L534 688H760L517 0Z" fill={RED} />
      </G>
      {cutColor ? <Rect x="71.68" y="245.76" width="368.64" height="20.48" fill={cutColor} /> : null}
    </Svg>
  );
}
