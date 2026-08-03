import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, ScrollView, Alert, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useSyncPlay } from "../../src/hooks/useSyncPlay";
import { useTheme } from "../../src/providers/ThemeProvider";

// Mobile parity with web /syncplay: join an existing Watch Together / Listen
// Together room by 5-digit code. Room *creation* on mobile goes through the
// player's SyncPlayModal (you start a media first, then invite others) since
// most mobile playback is media-first; the dedicated route is the explicit
// "I have a code, get me in" entry point.
export default function SyncPlayScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [code, setCode] = useState("");
  const [joining, setJoining] = useState(false);
  const sync = useSyncPlay();

  async function handleJoin() {
    const trimmed = code.trim();
    if (trimmed.length < 5) {
      Alert.alert(t("media.syncplay.joinByCode", "Join with code"), t("media.syncplay.invalidCode", "Enter the 5-digit room code."));
      return;
    }
    setJoining(true);
    try {
      const { mediaId } = await sync.joinByCode(trimmed);
      if (!mediaId) {
        Alert.alert(t("media.syncplay.title", "Watch Together"), t("media.syncplay.joinFailed", "Could not join the room."));
        return;
      }
      router.replace(`/player/${mediaId}` as never);
    } catch {
      Alert.alert(t("media.syncplay.title", "Watch Together"), t("media.syncplay.joinFailed", "Could not join the room."));
    } finally {
      setJoining(false);
    }
  }

  const s = makeStyles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={s.content}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("common.nav.syncplay", "SyncPlay"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      <View style={s.hero}>
        <Ionicons name="people-outline" size={40} color={theme.colors.accent} />
        <Text style={s.title}>{t("media.syncplay.title", "Watch Together")}</Text>
        <Text style={s.description}>
          {t(
            "media.syncplay.joinDescription",
            "Got a code from a friend? Enter it below to join their room. To start a new room, open a movie or song and tap the SyncPlay button.",
          )}
        </Text>
      </View>

      <Text style={s.label}>{t("media.syncplay.joinByCode", "Join with code")}</Text>
      <View style={s.row}>
        <TextInput
          value={code}
          onChangeText={setCode}
          placeholder="12345"
          placeholderTextColor={theme.colors.textMuted}
          keyboardType="number-pad"
          maxLength={5}
          editable={!joining}
          style={s.input}
          accessibilityLabel={t("media.syncplay.codeInput", "Room code")}
        />
        <Pressable
          onPress={handleJoin}
          disabled={joining || code.trim().length < 5}
          style={[s.btn, (joining || code.trim().length < 5) && s.btnDisabled]}
          accessibilityRole="button"
          accessibilityLabel={t("media.syncplay.join", "Join")}
        >
          {joining ? (
            <ActivityIndicator color={theme.colors.onAccent} size="small" />
          ) : (
            <Text style={s.btnText}>{t("media.syncplay.join", "Join")}</Text>
          )}
        </Pressable>
      </View>
    </ScrollView>
  );
}

function makeStyles(t: ReturnType<typeof useTheme>) {
  return StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    content: { padding: t.spacing.md, gap: t.spacing.md, paddingBottom: t.spacing.xl },
    hero: {
      alignItems: "center",
      gap: t.spacing.sm,
      paddingVertical: t.spacing.xl,
      paddingHorizontal: t.spacing.md,
    },
    title: {
      color: t.colors.text,
      fontSize: 22,
      fontWeight: "700",
      textAlign: "center",
    },
    description: {
      color: t.colors.textMuted,
      fontSize: 14,
      textAlign: "center",
      lineHeight: 20,
    },
    label: {
      color: t.colors.textMuted,
      fontSize: 13,
      fontWeight: "600",
      textTransform: "uppercase",
    },
    row: {
      flexDirection: "row",
      gap: t.spacing.sm,
    },
    input: {
      flex: 1,
      backgroundColor: t.colors.surface,
      color: t.colors.text,
      borderRadius: t.radii.md,
      paddingHorizontal: t.spacing.md,
      paddingVertical: t.spacing.sm + 4,
      borderWidth: 1,
      borderColor: t.colors.border,
      fontSize: 22,
      fontWeight: "700",
      letterSpacing: 4,
      textAlign: "center",
    },
    btn: {
      backgroundColor: t.colors.accent,
      borderRadius: t.radii.md,
      paddingHorizontal: t.spacing.lg,
      alignItems: "center",
      justifyContent: "center",
      minWidth: 96,
    },
    btnDisabled: { opacity: 0.5 },
    btnText: {
      color: t.colors.onAccent,
      fontSize: 15,
      fontWeight: "600",
    },
  });
}
