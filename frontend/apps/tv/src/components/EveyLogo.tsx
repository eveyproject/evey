import Svg, { Defs, G, Mask, Path, Rect } from "react-native-svg";

// The brand wordmark and mark, ported from the web component
// (apps/web/src/components/ui/EveyLogo.tsx) so the three clients draw the same
// letters from the same path data.
//
// Kept as vector rather than a PNG for the reason the web version is vector:
// the letters follow the theme text colour while the V stays brand red, and a
// television renders it at whatever size the layout asks for.
//
// The horizontal bar through the letters is a MASK CUT, not a drawn line - it
// shows the surface behind it. Filling it with the background colour would
// look right on a flat header and wrong over a poster.

const RED = "#DC2626";

interface LogoProps {
  height?: number;
  /** Letter colour. Defaults to white; pass the theme's text colour. */
  color?: string;
}

/** Full "EVEY" wordmark - letters take `color`, the V is brand red. */
export function EveyWordmark({ height = 26, color = "#FFFFFF" }: LogoProps) {
  // 3420 x 808 viewBox, offset by -60 like the web version.
  const width = (height * 3420) / 808;
  return (
    <Svg viewBox="-60 -60 3420 808" height={height} width={width} accessibilityLabel="Evey">
      <Defs>
        <Mask id="eveyWordmarkCut">
          <Rect width="100%" height="100%" fill="white" />
          <Rect x="0" y="304" width="3300" height="80" fill="black" />
        </Mask>
      </Defs>
      <G mask="url(#eveyWordmarkCut)">
        <Path d="M74 688H669V523H295V428H615V270H295V165H676V0H74Z" fill={color} transform="translate(0, 688) scale(1, -1)" />
        <Path d="M261 0 17 688H253L391 228H395L534 688H760L517 0Z" fill={RED} transform="translate(822, 688) scale(1, -1)" />
        <Path d="M74 688H669V523H295V428H615V270H295V165H676V0H74Z" fill={color} transform="translate(1700, 688) scale(1, -1)" />
        <Path d="M501 0H280V267L7 688H261L393 459H397L529 688H770L501 267Z" fill={color} transform="translate(2522, 688) scale(1, -1)" />
      </G>
    </Svg>
  );
}

/** Compact mark - the brand red V with the signature bar cut. */
export function EveyMark({ height = 28 }: { height?: number }) {
  return (
    <Svg viewBox="0 0 512 512" height={height} width={height} accessibilityLabel="Evey">
      <Defs>
        <Mask id="eveyMarkCut">
          <Rect width="512" height="512" fill="white" />
          <Rect x="71.68" y="245.76" width="368.64" height="20.48" fill="black" />
        </Mask>
      </Defs>
      <G mask="url(#eveyMarkCut)">
        <G transform="translate(70.72744186046509, 419.84) scale(0.4762790697674419, -0.4762790697674419)">
          <Path d="M261 0 17 688H253L391 228H395L534 688H760L517 0Z" fill={RED} />
        </G>
      </G>
    </Svg>
  );
}
