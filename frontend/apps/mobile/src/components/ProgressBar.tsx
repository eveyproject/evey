import { memo } from "react";
import { View } from "react-native";
import { useTheme } from "../providers/ThemeProvider";

interface ProgressBarProps {
  percent: number;
  color?: string;
}

function ProgressBarImpl({ percent, color }: ProgressBarProps) {
  const theme = useTheme();
  const barColor = color ?? theme.colors.accent;
  const clamped = Math.min(100, Math.max(0, percent));

  return (
    <View style={{ height: 3, backgroundColor: theme.colors.divider, borderRadius: 2 }}>
      <View style={{ height: 3, width: `${clamped}%`, backgroundColor: barColor, borderRadius: 2 }} />
    </View>
  );
}

// memo: primitive props (percent number, optional color string); rendered
// in every media card row, so the list-scroll path benefits the most.
export const ProgressBar = memo(ProgressBarImpl);
