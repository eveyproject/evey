import React, { useState, useCallback } from "react";
import { Pressable, StyleSheet, type ViewStyle, type StyleProp } from "react-native";
import { useTheme } from "../providers/ThemeProvider";

interface FocusableCardProps {
  onPress?: () => void;
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  focusedStyle?: StyleProp<ViewStyle>;
  hasTVPreferredFocus?: boolean;
}

export const FocusableCard = React.memo(function FocusableCard({
  onPress,
  children,
  style,
  focusedStyle,
  hasTVPreferredFocus,
}: FocusableCardProps) {
  const [focused, setFocused] = useState(false);
  const theme = useTheme();

  const handleFocus = useCallback(() => setFocused(true), []);
  const handleBlur = useCallback(() => setFocused(false), []);

  return (
    <Pressable
      onPress={onPress}
      onFocus={handleFocus}
      onBlur={handleBlur}
      hasTVPreferredFocus={hasTVPreferredFocus}
      style={[
        styles.card,
        style,
        focused && [styles.focused, { borderColor: theme.colors.accent }],
        focused && focusedStyle,
      ]}
      accessible={true}
      accessibilityRole="button"
    >
      {children}
    </Pressable>
  );
});

const styles = StyleSheet.create({
  card: {
    borderWidth: 3,
    borderColor: "transparent",
    borderRadius: 8,
  },
  focused: {
    transform: [{ scale: 1.05 }],
  },
});
