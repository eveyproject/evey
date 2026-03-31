import React, { useState, useCallback } from "react";
import { Pressable, StyleSheet, type ViewStyle, type StyleProp } from "react-native";

interface FocusableCardProps {
  onPress?: () => void;
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  focusedStyle?: StyleProp<ViewStyle>;
  hasTVPreferredFocus?: boolean;
}

export function FocusableCard({
  onPress,
  children,
  style,
  focusedStyle,
  hasTVPreferredFocus,
}: FocusableCardProps) {
  const [focused, setFocused] = useState(false);

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
        focused && styles.focused,
        focused && focusedStyle,
      ]}
      accessible={true}
      accessibilityRole="button"
    >
      {children}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  card: {
    borderWidth: 3,
    borderColor: "transparent",
    borderRadius: 8,
  },
  focused: {
    borderColor: "#6366f1",
    transform: [{ scale: 1.05 }],
  },
});
