import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, ScrollView, Alert, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import type { AudioPlayer } from "expo-audio";
import { normalizeRoomCode, ROOM_CODE_LENGTH, useLibraryServiceListLibraries } from "@evey/api";
import type { RoomQueueItem } from "@evey/client";
import { useSyncPlay } from "../../src/hooks/useSyncPlay";
import { useTheme } from "../../src/providers/ThemeProvider";
import { useMusicQueueStore } from "../../src/stores/musicQueue";
import { RoomBuilder } from "../../src/features/syncplay/RoomBuilder";
import { ListenTogetherRoom } from "../../src/features/syncplay/ListenTogetherRoom";

// Mobile parity with web /syncplay: join a Watch Together / Listen Together
// room by its share code, or build a "listen together" music room from albums,
// tracks and playlists and listen in it. A video room is joined here and
// watched in the player; a music room is listened to on this screen.
export default function SyncPlayScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [code, setCode] = useState("");
  const [joining, setJoining] = useState(false);
  const [creating, setCreating] = useState(false);
  const [musicRoom, setMusicRoom] = useState(false);
  const { data: libs } = useLibraryServiceListLibraries();
  const musicLibraryId = useMemo(() => libs?.libraries?.find((l) => l.type === "LIBRARY_TYPE_MUSIC")?.id, [libs]);

  // The room moves this player (a member follows the host). Applying a room
  // command must not be mistaken for a local action: local actions are sent
  // explicitly by the room controls, so the callbacks only move the player.
  const playerRef = useRef<AudioPlayer | null>(null);
  const trackMeta = useRef<Map<string, RoomQueueItem>>(new Map());
  const sync = useSyncPlay({
    onPlay: (ms) => {
      const p = playerRef.current;
      if (!p) return;
      void p.seekTo(ms / 1000);
      p.play();
    },
    onPause: (ms) => {
      const p = playerRef.current;
      if (!p) return;
      p.pause();
      void p.seekTo(ms / 1000);
    },
    onSeek: (ms) => { void playerRef.current?.seekTo(ms / 1000); },
  });

  // One music output at a time: the room takes over from the mini-player.
  useEffect(() => {
    if (musicRoom) useMusicQueueStore.setState({ isPlaying: false });
  }, [musicRoom]);

  const handleCreate = useCallback(async (items: RoomQueueItem[]) => {
    if (items.length === 0) return;
    setCreating(true);
    for (const it of items) trackMeta.current.set(it.id, it);
    try {
      await sync.createRoomFromQueue(items.map((it) => it.id));
      setMusicRoom(true);
    } catch {
      Alert.alert(t("media.syncplay.title", "Watch Together"), t("media.syncplay.createFailed", "Failed to create room"));
    } finally {
      setCreating(false);
    }
  }, [sync, t]);

  const handleLeave = useCallback(async () => {
    try { playerRef.current?.pause(); } catch { /* released */ }
    await sync.leaveRoom();
    setMusicRoom(false);
    setCode("");
  }, [sync]);

  async function handleJoin() {
    const trimmed = code.trim();
    if (trimmed.length < 5) {
      Alert.alert(t("media.syncplay.joinByCode", "Join with code"), t("media.syncplay.invalidCode", "Enter the 6-character room code."));
      return;
    }
    setJoining(true);
    try {
      const { mediaId, mediaType } = await sync.joinByCode(trimmed);
      if (mediaType === "music") {
        setMusicRoom(true);
        return;
      }
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

  if (musicRoom && sync.state.roomId) {
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
        <ListenTogetherRoom
          state={sync.state}
          controls={sync}
          playerRef={playerRef}
          trackMeta={trackMeta}
          onLeave={() => { void handleLeave(); }}
        />
      </ScrollView>
    );
  }

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
          onChangeText={(v) => setCode(normalizeRoomCode(v))}
          placeholder="ABC123"
          placeholderTextColor={theme.colors.textMuted}
          autoCapitalize="characters"
          autoCorrect={false}
          maxLength={ROOM_CODE_LENGTH}
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

      {musicLibraryId && (
        <View style={{ gap: theme.spacing.sm, marginTop: theme.spacing.lg }}>
          <Text style={s.label}>{t("media.syncplay.create", "Create a room")}</Text>
          <RoomBuilder libraryId={musicLibraryId} creating={creating} onCreate={(items) => { void handleCreate(items); }} />
        </View>
      )}
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
