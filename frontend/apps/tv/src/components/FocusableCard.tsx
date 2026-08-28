import React, { useState, useCallback } from "react";
import { Pressable, StyleSheet, type ViewStyle, type StyleProp, type AccessibilityState } from "react-native";
import { useTheme } from "../providers/ThemeProvider";

interface FocusableCardProps {
  onPress?: () => void;
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  focusedStyle?: StyleProp<ViewStyle>;
  hasTVPreferredFocus?: boolean;
  accessibilityLabel?: string;
  // accessibilityState conveys selected/disabled to the screen reader - a
  // colour-only "active" chip is invisible without it. Optional, so every
  // existing call site is unchanged.
  accessibilityState?: AccessibilityState;
  // disabled stops the Pressable AND tells the screen reader, so an in-flight
  // action cannot be re-fired by a second click of the remote. Merged into
  // accessibilityState rather than overriding whatever the caller passed.
  disabled?: boolean;
  // Focus passthrough for callers that must react to the remote landing here -
  // e.g. the home hero pausing its auto-rotation while its Play button holds
  // focus, so Play always acts on the title on screen. Optional: existing call
  // sites are unchanged.
  onFocus?: () => void;
  onBlur?: () => void;
}

export const FocusableCard = React.memo(function FocusableCard({
  onPress,
  children,
  style,
  focusedStyle,
  hasTVPreferredFocus,
  accessibilityLabel,
  accessibilityState,
  disabled,
  onFocus,
  onBlur,
}: FocusableCardProps) {
  const [focused, setFocused] = useState(false);
  const theme = useTheme();

  const handleFocus = useCallback(() => { setFocused(true); onFocus?.(); }, [onFocus]);
  const handleBlur = useCallback(() => { setFocused(false); onBlur?.(); }, [onBlur]);

  return (
    <Pressable
      onPress={onPress}
      onFocus={handleFocus}
      onBlur={handleBlur}
      disabled={disabled}
      hasTVPreferredFocus={hasTVPreferredFocus}
      style={[
        styles.card,
        style,
        focused && [styles.focused, { borderColor: theme.colors.accent }],
        focused && focusedStyle,
      ]}
      accessible={true}
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      accessibilityState={disabled ? { ...accessibilityState, disabled: true } : accessibilityState}
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
