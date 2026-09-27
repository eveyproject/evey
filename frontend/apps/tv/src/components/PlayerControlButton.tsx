import { useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";

// A player control the remote can land on. The overlay used to show three
// silent circles: nothing said which one was selected, so the viewer pressed
// OK and hoped. `focusable` is false while the surface owns the D-pad, which
// keeps left/right seeking instead of walking this row.
export function PlayerControlButton({
  label,
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
  label: string;
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
  const [focused, setFocused] = useState(false);
  const size = small ? 56 : primary ? 104 : 84;
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
        styles.controlButton,
        { width: size, height: size, borderRadius: size / 2 },
        focused && styles.controlButtonFocused,
      ]}
    >
      <Text style={[styles.controlLabel, { fontSize: small ? 26 : primary ? 40 : 22 }, focused && styles.controlLabelFocused]}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  // Bigger and closer together than the old silent circles, and sitting in
  // the middle of the screen rather than floating above the action bar.
  controlButton: {
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "rgba(255,255,255,0.18)",
    borderWidth: 3,
    borderColor: "transparent",
  },
  // The selected control is unmistakable from the sofa: filled, outlined, dark
  // glyph. A focus ring alone was invisible on a bright frame.
  controlButtonFocused: {
    backgroundColor: "rgba(255,255,255,0.92)",
    borderColor: "#fff",
  },
  controlLabel: { color: "#fff", fontWeight: "700" },
  controlLabelFocused: { color: "#111" },
});
