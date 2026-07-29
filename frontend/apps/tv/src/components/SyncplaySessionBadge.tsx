import { useCallback, useState } from "react";
import { Pressable, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";

interface SyncplaySessionBadgeProps {
  connected: boolean;
  // Not connected: opens the join screen. Connected: leaves the room. One
  // button, one focus target - see the player screen's D-pad handler for why
  // this needs its own onFocusChange guard.
  onPress: () => void;
  // The player's global useTVEventHandler receives every raw "select" key
  // press regardless of native TV focus, so while this badge is focused a
  // bare OK press would ALSO fire togglePlayPause unless the screen guards
  // it - same double-fire this app already guards for SkipIntroButton.
  onFocusChange?: (focused: boolean) => void;
}

// A single, always-present affordance in the player's top bar: "Watch
// Together" before a session, a discreet "Synced" pill (that also leaves)
// once connected. Deliberately not built on the shared FocusableCard (it
// does not expose focus state to its caller) - this is its own minimal
// Pressable so the player screen can guard the D-pad "select" double-fire
// above.
export function SyncplaySessionBadge({ connected, onPress, onFocusChange }: SyncplaySessionBadgeProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [focused, setFocused] = useState(false);

  const handleFocus = useCallback(() => {
    setFocused(true);
    onFocusChange?.(true);
  }, [onFocusChange]);
  const handleBlur = useCallback(() => {
    setFocused(false);
    onFocusChange?.(false);
  }, [onFocusChange]);

  const label = connected
    ? t("media.syncplay.followingHost", "You're synced with the host.")
    : t("media.syncplay.join", "Join a room");

  return (
    <Pressable
      onPress={onPress}
      onFocus={handleFocus}
      onBlur={handleBlur}
      accessible
      accessibilityRole="button"
      accessibilityLabel={label}
      style={[
        styles.badge,
        {
          backgroundColor: theme.colors.overlay,
          borderColor: focused ? theme.colors.accent : connected ? theme.colors.success : "transparent",
        },
      ]}
    >
      <Text style={[styles.text, { color: theme.colors.text }]} numberOfLines={1}>
        {connected ? t("media.syncplay.connected", "Connected") : label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  badge: {
    borderWidth: 2,
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 4,
    maxWidth: 220,
  },
  text: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "600",
  },
});
