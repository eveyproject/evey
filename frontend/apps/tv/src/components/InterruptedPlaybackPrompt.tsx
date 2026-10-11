import { View, Text, Modal, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { formatTimeMs } from "@evey/api";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import type { InterruptedPlayback } from "../lib/interruptedPlayback";

interface InterruptedPlaybackPromptProps {
  mark: InterruptedPlayback | null;
  onResume: () => void;
  onDismiss: () => void;
}

/** "The film stopped, carry on?" - once, after the system killed the player. Resume holds the focus: one press. */
export function InterruptedPlaybackPrompt({ mark, onResume, onDismiss }: InterruptedPlaybackPromptProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  if (!mark) return null;

  const at = mark.positionMs > 0 ? formatTimeMs(mark.positionMs) : "";
  return (
    <Modal visible transparent animationType="fade" onRequestClose={onDismiss}>
      <View style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <View style={[styles.card, { backgroundColor: theme.colors.surface, borderColor: theme.colors.border }]}>
          <Text style={[styles.heading, { color: theme.colors.textMuted }]}>
            {t("player.interruptedResume.heading", "Playback was interrupted")}
          </Text>
          {mark.title ? (
            <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
              {mark.title}
            </Text>
          ) : null}
          <View style={styles.actions}>
            <FocusableCard
              hasTVPreferredFocus={true}
              onPress={onResume}
              accessibilityLabel={at ? t("player.interruptedResume.resumeAt", { time: at }) : t("player.interruptedResume.resume", "Resume")}
              style={[styles.button, { backgroundColor: theme.colors.accent }]}
            >
              <Text style={[styles.resumeText, { color: theme.colors.onAccent }]}>
                {at ? t("player.interruptedResume.resumeAt", { time: at }) : t("player.interruptedResume.resume", "Resume")}
              </Text>
            </FocusableCard>
            <FocusableCard
              onPress={onDismiss}
              accessibilityLabel={t("player.interruptedResume.dismiss", "Not now")}
              style={[styles.button, { backgroundColor: theme.colors.bg }]}
            >
              <Text style={[styles.dismissText, { color: theme.colors.text }]}>{t("player.interruptedResume.dismiss", "Not now")}</Text>
            </FocusableCard>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  card: {
    width: 560,
    borderRadius: 16,
    borderWidth: 1,
    padding: 28,
  },
  heading: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "700",
    marginTop: 6,
    marginBottom: 20,
  },
  actions: {
    flexDirection: "row",
    gap: 12,
  },
  button: {
    flex: 1,
    paddingVertical: 10,
    alignItems: "center",
    justifyContent: "center",
  },
  resumeText: {
    fontWeight: "700",
    fontSize: FONT_SIZE.base,
  },
  dismissText: {
    fontWeight: "600",
    fontSize: FONT_SIZE.base,
  },
});
