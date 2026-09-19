import React from "react";
import { View, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";

// What a screen under this button must keep clear at its bottom edge: the
// button (56) plus its margin (24) plus a little air. It floats over the
// content, and without the inset it sat ON it - a covered title in the
// history, a covered poster on a media page, a covered genre in the stats
// (seen on the living-room box, 2026-09-19).
export const HOME_SHORTCUT_INSET = 88;

interface HomeShortcutButtonProps {
  onPress: () => void;
}

// Floating, always-focusable way back to Home from a deep screen (media/
// person/library detail, collections, playlists, settings sub-pages...)
// that renders full-screen with no sidebar. A corner overlay rather than a
// remote-key reinterpretation: it never competes with the plain Back key,
// which keeps popping the stack one level as every Android TV user expects.
//
// No hasTVPreferredFocus here - the screen underneath owns its own initial
// focus target, this button must never steal it.
export function HomeShortcutButton({ onPress }: HomeShortcutButtonProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  return (
    <View style={styles.container} pointerEvents="box-none">
      <FocusableCard
        onPress={onPress}
        accessibilityLabel={t("common.nav.home")}
        style={[
          styles.button,
          { backgroundColor: theme.colors.surface, borderColor: theme.colors.border },
        ]}
      >
        <Ionicons name="home" size={28} color={theme.colors.accent} />
      </FocusableCard>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    position: "absolute",
    left: 24,
    bottom: 24,
    zIndex: 10,
  },
  button: {
    width: 56,
    height: 56,
    borderRadius: 28,
    borderWidth: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});
