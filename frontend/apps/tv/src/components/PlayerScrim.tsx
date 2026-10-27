import { memo } from "react";
import { StyleSheet, View } from "react-native";
import Svg, { Defs, LinearGradient, Rect, Stop } from "react-native-svg";

// The fade the web player draws behind its bars (overlay-bg-strong to
// transparent): the controls read on any frame and the picture between them
// stays the picture, with no hard edge where a flat band used to stop.
export const PlayerScrim = memo(function PlayerScrim({ edge, opacity = 0.85 }: { edge: "top" | "bottom"; opacity?: number }) {
  const id = `player-scrim-${edge}`;
  return (
    <View style={StyleSheet.absoluteFill} pointerEvents="none">
      <Svg width="100%" height="100%">
        <Defs>
          <LinearGradient id={id} x1="0" y1={edge === "top" ? "0" : "1"} x2="0" y2={edge === "top" ? "1" : "0"}>
            <Stop offset="0" stopColor="#000" stopOpacity={opacity} />
            <Stop offset="0.55" stopColor="#000" stopOpacity={opacity * 0.55} />
            <Stop offset="1" stopColor="#000" stopOpacity={0} />
          </LinearGradient>
        </Defs>
        <Rect x="0" y="0" width="100%" height="100%" fill={`url(#${id})`} />
      </Svg>
    </View>
  );
});
