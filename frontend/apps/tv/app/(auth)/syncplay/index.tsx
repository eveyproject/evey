import { useCallback, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { joinSyncplayRoomByCode } from "../../../src/hooks/useSyncplay";
import { isValidRoomCode, ROOM_CODE_LENGTH, ROOM_CODE_ALPHABET } from "../../../src/lib/syncplayReconcile";
import { FONT_SIZE } from "@evey/theme";

// TV is a JOIN-only SyncPlay entry point, deliberately: creating a room means
// sharing its code with other viewers, and that sharing (a screenshot, a text
// message, reading it out over the phone) is something a phone does far better
// than a TV - web and mobile already offer room creation.
//
// The grid below is the alphabet, not a numeric keypad: the code gained letters
// when 5 digits proved enumerable, and a remote cannot paste. Six columns so a
// row is one D-pad sweep, digits first as they are in the alphabet itself.
//
// The room membership itself is granted by this REST call server-side (see
// syncplay_handler.go's JoinByCode, which calls hub.JoinRoom before this
// screen ever opens a websocket) - this screen only resolves the code to a
// room/media pair and hands off; the actual live sync connection is opened
// by the player screen's own useSyncplay instance once it mounts with the
// resulting media, via the `roomId` navigation param.
const KEYS_PER_ROW = 6;

const KEYPAD_ROWS: (string | null)[][] = (() => {
  const keys: string[] = [...ROOM_CODE_ALPHABET, "del"];
  const rows: (string | null)[][] = [];
  for (let i = 0; i < keys.length; i += KEYS_PER_ROW) {
    const row: (string | null)[] = keys.slice(i, i + KEYS_PER_ROW);
    while (row.length < KEYS_PER_ROW) row.push(null);
    rows.push(row);
  }
  return rows;
})();

export default function SyncplayJoinScreen() {
  const { t } = useTranslation();
  const theme = useTheme();
  const [code, setCode] = useState("");
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = useCallback(
    async (candidate: string) => {
      if (!isValidRoomCode(candidate) || joining) return;
      setJoining(true);
      setError(null);
      try {
        const { roomId, mediaId } = await joinSyncplayRoomByCode(candidate);
        router.replace(`/(auth)/player/${mediaId}?roomId=${roomId}` as never);
      } catch {
        setError(t("media.syncplay.joinError", "Could not join - check the code."));
        setCode("");
        setJoining(false);
      }
    },
    [joining, t],
  );

  function handleKeyPress(key: string) {
    if (joining) return;
    if (key === "del") {
      setCode((prev) => prev.slice(0, -1));
      if (error) setError(null);
      return;
    }
    setCode((prev) => {
      if (prev.length >= ROOM_CODE_LENGTH) return prev;
      const next = prev + key;
      if (next.length === ROOM_CODE_LENGTH) void handleSubmit(next);
      return next;
    });
    if (error) setError(null);
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.title, { color: theme.colors.text }]}>
        {t("media.syncplay.join", "Join a room")}
      </Text>
      <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
        {t("media.syncplay.joinByCode", "Join with code")}
      </Text>

      <View style={styles.dots}>
        {Array.from({ length: ROOM_CODE_LENGTH }).map((_, i) => (
          <View
            key={i}
            style={[
              styles.dot,
              {
                borderColor: error ? theme.colors.error : theme.colors.border,
                backgroundColor: i < code.length ? theme.colors.accent : "transparent",
              },
            ]}
          />
        ))}
      </View>

      {joining ? (
        <ActivityIndicator color={theme.colors.accent} size="small" style={styles.status} />
      ) : error ? (
        <Text style={[styles.error, { color: theme.colors.error }]}>{error}</Text>
      ) : (
        <View style={styles.status} />
      )}

      <View style={styles.keypad}>
        {KEYPAD_ROWS.map((row, ri) => (
          <View key={ri} style={styles.keypadRow}>
            {row.map((key, ki) =>
              key === null ? (
                <View key={ki} style={styles.key} />
              ) : (
                <FocusableCard
                  key={ki}
                  hasTVPreferredFocus={ri === 0 && ki === 0}
                  onPress={() => handleKeyPress(key)}
                  disabled={joining}
                  accessibilityLabel={key === "del" ? t("common.delete", "Delete") : key}
                  style={[styles.key, { backgroundColor: theme.colors.surface }]}
                >
                  <Text style={[styles.keyText, { color: theme.colors.text }]}>
                    {key === "del" ? "⌫" : key}
                  </Text>
                </FocusableCard>
              ),
            )}
          </View>
        ))}
      </View>

      <Pressable
        onPress={() => router.back()}
        accessible
        accessibilityRole="button"
        accessibilityLabel={t("common.back", "Back")}
        style={styles.backBtn}
      >
        <Text style={[styles.backText, { color: theme.colors.accent }]}>{t("common.back", "Back")}</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    padding: 80,
  },
  title: {
    fontSize: FONT_SIZE["3xl"],
    fontWeight: "bold",
    marginBottom: 8,
  },
  desc: {
    fontSize: FONT_SIZE.base,
    marginBottom: 24,
  },
  dots: {
    flexDirection: "row",
    gap: 16,
    marginBottom: 8,
  },
  dot: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
  },
  status: {
    height: 24,
    marginTop: 8,
    marginBottom: 8,
  },
  error: {
    fontSize: FONT_SIZE.sm,
    textAlign: "center",
    marginTop: 8,
    marginBottom: 8,
  },
  keypad: {
    gap: 10,
    marginTop: 16,
  },
  keypadRow: {
    flexDirection: "row",
    gap: 10,
  },
  key: {
    width: 72,
    height: 56,
    borderRadius: 8,
    alignItems: "center",
    justifyContent: "center",
  },
  keyText: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "600",
  },
  backBtn: {
    marginTop: 32,
    paddingVertical: 12,
    paddingHorizontal: 32,
  },
  backText: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
  },
});
