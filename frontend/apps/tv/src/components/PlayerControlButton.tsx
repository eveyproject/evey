import { memo, useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";

// A player control the remote can land on. The overlay used to show three
// silent circles: nothing said which one was selected, so the viewer pressed
// OK and hoped. `focusable` is false while the surface owns the D-pad, which
// keeps left/right seeking instead of walking this row.
//
// Evey's look, as on the web player: white glyphs on the picture, the brand
// accent where the focus is. The lit control fills with the accent - a ring
// alone was invisible on a bright frame from the sofa.
export const PlayerControlButton = memo(function PlayerControlButton({
  icon,
  text,
  caption,
  onPress,
  accessibilityLabel,
  focusable,
  buttonRef,
  primary,
  small,
  nextFocusLeft,
  nextFocusRight,
  nextFocusDown,
  nextFocusUp,
}: {
  // An icon, or a short text for a value (the speed) that no icon can say.
  icon?: React.ComponentProps<typeof Ionicons>["name"];
  text?: string;
  // A small line under the icon: the seconds a skip button jumps.
  caption?: string;
  onPress: () => void;
  accessibilityLabel: string;
  focusable: boolean;
  // Explicit neighbours. Left to itself, Android's geometric search sent the
  // remote from the play button to the syncplay badge at the top of the
  // screen (measured on the box, 2026-09-19); these four keep the row and the
  // settings icon talking to each other.
  nextFocusLeft?: number;
  nextFocusRight?: number;
  nextFocusDown?: number;
  nextFocusUp?: number;
  // Handed the node so the screen can give it the focus when the row appears:
  // hasTVPreferredFocus is read at MOUNT, and these buttons are mounted long
  // before they become focusable (measured on the box, 2026-09-19 - the row
  // showed up with nothing lit).
  buttonRef?: React.RefObject<View | null>;
  primary?: boolean;
  small?: boolean;
}) {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  const size = small ? 60 : primary ? 96 : 76;
  const iconSize = small ? 28 : primary ? 48 : caption ? 30 : 36;
  const color = focused ? theme.colors.onAccent : "#fff";
  return (
    <Pressable
      ref={buttonRef as never}
      focusable={focusable}
      nextFocusLeft={nextFocusLeft}
      nextFocusRight={nextFocusRight}
      nextFocusDown={nextFocusDown}
      nextFocusUp={nextFocusUp}
      onPress={onPress}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      accessible={true}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      style={[
        styles.button,
        { width: size, height: size, borderRadius: size / 2 },
        primary && styles.primary,
        focused && [styles.focused, { backgroundColor: theme.colors.accent }],
      ]}
    >
      {icon ? <Ionicons name={icon} size={iconSize} color={color} /> : null}
      {text ? <Text style={[styles.text, { color }]}>{text}</Text> : null}
      {caption ? <Text style={[styles.caption, { color }]}>{caption}</Text> : null}
    </Pressable>
  );
});

const styles = StyleSheet.create({
  button: { alignItems: "center", justifyContent: "center", backgroundColor: "transparent" },
  // The one control the viewer reaches for most keeps a soft disc at rest, so
  // the row has a centre before anything is lit.
  primary: { backgroundColor: "rgba(255,255,255,0.15)" },
  focused: { transform: [{ scale: 1.08 }] },
  text: { fontSize: FONT_SIZE.lg, fontWeight: "700", fontVariant: ["tabular-nums"] },
  caption: { fontSize: FONT_SIZE.xs, fontWeight: "700", marginTop: -2, fontVariant: ["tabular-nums"] },
});
