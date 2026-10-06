import { useCallback, useState } from "react";
import { Pressable, StyleSheet, Text } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { displayTitle, usePlaybackServiceMarkAsUnwatched, usePlaybackServiceMarkAsWatched, type V1MediaItem } from "@evey/api";
import { invalidateWatchedState } from "@evey/client";
import { toast, useAuthStore } from "@evey/store";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";
import { useIsFavorite, useToggleFavorite } from "../hooks/useFavorite";
import { BottomSheet } from "./BottomSheet";
import { AddToPlaylistSheet } from "./AddToPlaylistSheet";
import { BulkAddToCollectionSheet } from "./library/BulkAddSheets";

interface Action {
  key: string;
  icon: keyof typeof Ionicons.glyphMap;
  label: string;
  onPress: () => void;
}

// The long-press menu of a home card: the web card's context menu, same
// entries, same admin gates (MediaCardContextMenu.tsx).
export function MediaActionsSheet({ item, onClose }: { item: V1MediaItem | null; onClose: () => void }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const qc = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const mediaId = item?.id;
  const { data: isFav = false } = useIsFavorite(mediaId);
  const toggleFav = useToggleFavorite(mediaId);
  // Follow-up sheets keep their own item: the menu has closed by then.
  const [playlistFor, setPlaylistFor] = useState<V1MediaItem | null>(null);
  const [collectionFor, setCollectionFor] = useState<string | null>(null);

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: (_d, v) => { invalidateWatchedState(qc, v.mediaItemId); toast.success(t("media.contextMenu.markedWatched")); },
      onError: () => toast.error(t("media.contextMenu.markError", { status: "watched" })),
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: (_d, v) => { invalidateWatchedState(qc, v.mediaItemId); toast.success(t("media.contextMenu.markedUnwatched")); },
      onError: () => toast.error(t("media.contextMenu.markError", { status: "unwatched" })),
    },
  });

  // Close first, open the follow-up on the next tick: a second RN Modal
  // mounted in the commit that dismisses the first can be swallowed on iOS.
  const thenOpen = useCallback((fn: () => void) => {
    onClose();
    setTimeout(fn, 250);
  }, [onClose]);

  const mark = (watched: boolean) => {
    onClose();
    if (!mediaId) return;
    if (!profileId) { toast.error(t("media.contextMenu.noProfileSelected")); return; }
    (watched ? markWatched : markUnwatched).mutate({ mediaItemId: mediaId, params: { profileId } });
  };

  const actions: Action[] = !item || !mediaId ? [] : [
    ...(isAdmin ? [{ key: "edit-metadata", icon: "create-outline" as const, label: t("media.contextMenu.editMetadata"), onPress: () => { onClose(); router.push(`/media/${mediaId}?edit=1` as never); } }] : []),
    { key: "add-to-playlist", icon: "list-outline", label: t("media.contextMenu.addToPlaylist"), onPress: () => thenOpen(() => setPlaylistFor(item)) },
    ...(isAdmin ? [{ key: "add-to-collection", icon: "albums-outline" as const, label: t("media.contextMenu.addToCollection"), onPress: () => thenOpen(() => setCollectionFor(mediaId)) }] : []),
    {
      key: isFav ? "remove-from-favorites" : "add-to-favorites",
      icon: isFav ? "heart" : "heart-outline",
      label: isFav ? t("media.contextMenu.removeFromFavorites") : t("media.contextMenu.addToFavorites"),
      onPress: () => { toggleFav.mutate(isFav); onClose(); },
    },
    { key: "mark-watched", icon: "eye-outline", label: t("media.contextMenu.markWatched"), onPress: () => mark(true) },
    { key: "mark-unwatched", icon: "eye-off-outline", label: t("media.contextMenu.markUnwatched"), onPress: () => mark(false) },
  ];

  return (
    <>
      <BottomSheet visible={!!item} onClose={onClose} title={item ? displayTitle(item, t) : ""}>
        {actions.map((a) => (
          <Pressable
            key={a.key}
            onPress={a.onPress}
            style={[styles.row, { borderRadius: theme.radii.md }]}
            accessible={true}
            accessibilityRole="button"
            accessibilityLabel={a.label}
            testID={`media-actions-${a.key}`}
          >
            <Ionicons name={a.icon} size={20} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.sm }}>{a.label}</Text>
          </Pressable>
        ))}
      </BottomSheet>
      {playlistFor?.id && (
        <AddToPlaylistSheet visible mediaItemId={playlistFor.id} mediaType={playlistFor.type} onClose={() => setPlaylistFor(null)} />
      )}
      {collectionFor && (
        <BulkAddToCollectionSheet visible mediaItemIds={[collectionFor]} onClose={() => setCollectionFor(null)} />
      )}
    </>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", alignItems: "center", gap: 14, paddingHorizontal: 12, paddingVertical: 14 },
});
