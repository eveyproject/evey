import { useEffect, useRef } from "react";
import { Animated, type ViewStyle } from "react-native";
import { useTheme } from "../providers/ThemeProvider";

interface SkeletonProps {
  width: number | string;
  height: number | string;
  borderRadius?: number;
  style?: ViewStyle;
}

export function Skeleton({ width, height, borderRadius = 4, style }: SkeletonProps) {
  const theme = useTheme();
  const opacity = useRef(new Animated.Value(0.3)).current;

  useEffect(() => {
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, { toValue: 1, duration: 800, useNativeDriver: true }),
        Animated.timing(opacity, { toValue: 0.3, duration: 800, useNativeDriver: true }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [opacity]);

  return (
    <Animated.View
      style={[
        {
          width: width as number,
          height: height as number,
          borderRadius,
          backgroundColor: theme.colors.surface,
          opacity,
        },
        style,
      ]}
    />
  );
}
