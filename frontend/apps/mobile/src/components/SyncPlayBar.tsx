import { useState } from "react";
import { View, Text, Pressable, Share, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import type { SyncPlayMember } from "../hooks/useSyncPlay";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

interface Props {
  roomCode: string | null;
  isHost: boolean;
  members: SyncPlayMember[];
  connected: boolean;
  onLeave: () => void;
  queue?: string[];
  trackIndex?: number;
  onNext?: () => void;
  onPrev?: () => void;
}

export function SyncPlayBar({ roomCode, isHost, members, connected, onLeave, queue, trackIndex, onNext, onPrev }: Props) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [copied, setCopied] = useState(false);

  const names = members.slice(0, 3).map((m) => m.username);
  const extra = members.length > 3 ? members.length - 3 : 0;
  const label = names.length > 0
    ? extra > 0
      ? t("media.syncplay.watchingWithMore", { names: names.join(", "), count: extra })
      : t("media.syncplay.watchingWith", { names: names.join(", ") })
    : t("media.syncplay.watchTogetherRoom", "Watch together room");

  async function handleCopyCode() {
    if (!roomCode) return;
    try {
      await Share.share({ message: roomCode });
    } catch (err) { console.warn("caught", err); }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  const hasQueue = (queue?.length ?? 0) > 0;
  const idx = trackIndex ?? 0;
  const total = queue?.length ?? 0;

  return (
    <View style={[styles.bar, { backgroundColor: theme.colors.overlay }]}>
      <View style={[styles.dot, { backgroundColor: connected ? theme.colors.success : theme.colors.error }]} />
      <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={1}>{label}</Text>
      {hasQueue && isHost && (
        <>
          <Pressable
            onPress={onPrev}
            disabled={idx <= 0}
            style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised, opacity: idx <= 0 ? 0.4 : 1 }]}
            accessibilityLabel={t("common.previous", "Previous")}
          >
            <Ionicons name="play-skip-back" size={ICON_SIZE.sm} color={theme.colors.text} />
          </Pressable>
          <Text style={[styles.chipText, { color: theme.colors.text }]}>{idx + 1}/{total}</Text>
          <Pressable
            onPress={onNext}
            disabled={idx >= total - 1}
            style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised, opacity: idx >= total - 1 ? 0.4 : 1 }]}
            accessibilityLabel={t("common.next", "Next")}
          >
            <Ionicons name="play-skip-forward" size={ICON_SIZE.sm} color={theme.colors.text} />
          </Pressable>
        </>
      )}
      {hasQueue && !isHost && (
        <Text style={[styles.chipText, { color: theme.colors.textMuted }]}>{idx + 1}/{total}</Text>
      )}
      {isHost && roomCode && (
        <Pressable onPress={handleCopyCode} style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised }]}>
          <Text style={[styles.chipText, { color: theme.colors.text }]}>{copied ? t("common.copied", "Copied") : roomCode}</Text>
        </Pressable>
      )}
      <Pressable onPress={onLeave} style={[styles.chip, { backgroundColor: theme.colors.error + "4D" }]}>
        <Ionicons name="exit-outline" size={ICON_SIZE.sm} color={theme.colors.text} />
        <Text style={[styles.chipText, { color: theme.colors.text }]}>{t("media.syncplay.leave", "Leave")}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  dot: { width: 8, height: 8, borderRadius: 4 },
  label: { flex: 1, fontSize: FONT_SIZE.xs },
  chip: { flexDirection: "row", alignItems: "center", gap: 4, paddingHorizontal: 8, paddingVertical: 4, borderRadius: 4 },
  chipText: { fontSize: FONT_SIZE.xs, fontWeight: "600" },
});
