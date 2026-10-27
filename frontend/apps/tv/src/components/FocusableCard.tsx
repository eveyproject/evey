import React, { useState, useCallback } from "react";
import { Pressable, StyleSheet, View, type ViewStyle, type StyleProp, type AccessibilityState } from "react-native";
import { useTheme } from "../providers/ThemeProvider";
import { useLiteMode } from "../lib/liteMode";
import { FOCUS_RING_WIDTH } from "../lib/focusRing";

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
  // The light mode keeps the accent border and drops the zoom: scaling a card
  // re-rasterises it, poster included, on every focus move.
  const lite = useLiteMode();

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
        outerSize(style),
        focused && !lite && styles.focused,
        focused && focusedStyle,
      ]}
      accessible={true}
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      accessibilityState={disabled ? { ...accessibilityState, disabled: true } : accessibilityState}
    >
      {children}
      {/* Drawn OVER the children, not as the card's own border: a poster as
          wide as the card covered the border's left and right edges. Offset by
          the border width so it hugs the outer edge, with the card's radius. */}
      {focused && (
        <View
          pointerEvents="none"
          style={[styles.ring, { borderColor: theme.colors.accent, borderRadius: ringRadius(style) }]}
        />
      )}
    </Pressable>
  );
});

const BORDER_WIDTH = FOCUS_RING_WIDTH;
const DEFAULT_RADIUS = 8;

// Own flattening: the screens' tests mock react-native down to a few parts.
function flatten(style: StyleProp<ViewStyle>): ViewStyle {
  if (Array.isArray(style)) return Object.assign({}, ...(style as unknown[]).map((s) => flatten(s as StyleProp<ViewStyle>)));
  return style && typeof style === "object" ? (style as ViewStyle) : {};
}

function ringRadius(style: StyleProp<ViewStyle>): number {
  const r = flatten(style).borderRadius;
  return typeof r === "number" ? r : DEFAULT_RADIUS;
}

// A numeric size in the caller's style is the size of its content (a poster
// as wide as the card): the border is added outside it, or the content sat
// inside the left edge and past the right one.
function outerSize(style: StyleProp<ViewStyle>): ViewStyle | null {
  const flat = flatten(style);
  const out: ViewStyle = {};
  if (typeof flat.width === "number") out.width = flat.width + 2 * BORDER_WIDTH;
  if (typeof flat.height === "number") out.height = flat.height + 2 * BORDER_WIDTH;
  return out.width !== undefined || out.height !== undefined ? out : null;
}

const styles = StyleSheet.create({
  // The transparent border keeps the card's size whether or not it is
  // focused; the visible ring is the overlay below.
  card: {
    borderWidth: BORDER_WIDTH,
    borderColor: "transparent",
    borderRadius: DEFAULT_RADIUS,
  },
  ring: {
    position: "absolute",
    top: -BORDER_WIDTH,
    left: -BORDER_WIDTH,
    right: -BORDER_WIDTH,
    bottom: -BORDER_WIDTH,
    borderWidth: BORDER_WIDTH,
  },
  focused: {
    transform: [{ scale: 1.05 }],
  },
});
