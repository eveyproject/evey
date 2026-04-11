import { useState } from "react";
import { View, Text, Pressable, Share, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import type { SyncPlayMember } from "../hooks/useSyncPlay";

interface Props {
  roomCode: string | null;
  isHost: boolean;
  members: SyncPlayMember[];
  connected: boolean;
  onLeave: () => void;
}

export function SyncPlayBar({ roomCode, isHost, members, connected, onLeave }: Props) {
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
    } catch {}
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <View style={[styles.bar, { backgroundColor: theme.colors.overlay }]}>
      <View style={[styles.dot, { backgroundColor: connected ? theme.colors.success : theme.colors.error }]} />
      <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={1}>{label}</Text>
      {isHost && roomCode && (
        <Pressable onPress={handleCopyCode} style={[styles.chip, { backgroundColor: theme.colors.surfaceRaised }]}>
          <Text style={[styles.chipText, { color: theme.colors.text }]}>{copied ? t("common.copied", "Copied") : roomCode}</Text>
        </Pressable>
      )}
      <Pressable onPress={onLeave} style={[styles.chip, { backgroundColor: theme.colors.error + "4D" }]}>
        <Ionicons name="exit-outline" size={14} color={theme.colors.text} />
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
  label: { flex: 1, fontSize: 12 },
  chip: { flexDirection: "row", alignItems: "center", gap: 4, paddingHorizontal: 8, paddingVertical: 4, borderRadius: 4 },
  chipText: { fontSize: 11, fontWeight: "600" },
});
