import { useCallback } from "react";
import { View, Text, Pressable, ActivityIndicator, ScrollView, StyleSheet } from "react-native";
import type { StyleProp, ViewStyle } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic, displayTitle } from "@evey/api";
import type { V1MediaItem, V1Track } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { useServerUrl } from "../hooks/useServerUrl";
import { artworkUrl } from "../lib/artwork";
import { LoadFailed } from "./LoadFailed";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

type SearchKind = "media" | "music";

// Same i18n key namespace as TV's mediaTypeLabel.ts (frontend/apps/tv/src/lib/mediaTypeLabel.ts):
// stripping the enum prefix and lowercasing it shows raw English ("2024 · movie")
// to every viewer, in every language.
const ITEM_TYPE_KEYS: Record<string, string> = {
  MEDIA_ITEM_TYPE_MOVIE: "media.itemType.movie",
  MEDIA_ITEM_TYPE_SERIES: "media.itemType.series",
  MEDIA_ITEM_TYPE_SEASON: "media.itemType.season",
  MEDIA_ITEM_TYPE_EPISODE: "media.itemType.episode",
  MEDIA_ITEM_TYPE_ALBUM: "media.itemType.album",
  MEDIA_ITEM_TYPE_TRACK: "media.itemType.track",
  MEDIA_ITEM_TYPE_ARTIST: "media.itemType.artist",
  MEDIA_ITEM_TYPE_PHOTO: "media.itemType.photo",
  MEDIA_ITEM_TYPE_BOOK: "media.itemType.book",
};

interface SearchDropdownProps {
  /** Absolute position (top/left/width) supplied by the caller, which
   * measures the header input in window coordinates since this panel
   * renders inside a Modal - a separate native surface from the header. */
  style?: StyleProp<ViewStyle>;
  /** Already debounced; trimmed here so callers don't have to. */
  query: string;
  visible: boolean;
  kind: SearchKind;
  onKindChange: (kind: SearchKind) => void;
  recents: string[];
  onPickRecent: (query: string) => void;
  onRemoveRecent: (query: string) => void;
  onClearRecents: () => void;
  onSelectMedia: (item: V1MediaItem) => void;
  onSelectTrack: (track: V1Track) => void;
  onSeeAll: () => void;
}

export function SearchDropdown({
  style,
  query,
  visible,
  kind,
  onKindChange,
  recents,
  onPickRecent,
  onRemoveRecent,
  onClearRecents,
  onSelectMedia,
  onSelectTrack,
  onSeeAll,
}: SearchDropdownProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const trimmed = query.trim();

  // Only the active scope queries - same reasoning as web's SearchDropdown.
  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia(
    { query: trimmed, limit: 6 },
    { query: { enabled: visible && trimmed.length > 1 && kind === "media" } },
  );
  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;

  const { data: musicData, isLoading: musicLoading, isError: musicError, refetch: refetchMusic } = useMusicServiceSearchMusic(
    { query: trimmed, limit: 6 },
    { query: { enabled: visible && trimmed.length > 1 && kind === "music" } },
  );
  const musicItems = musicData?.tracks ?? [];

  const activeLoading = kind === "media" ? isLoading : musicLoading;
  const activeError = kind === "media" ? isError : musicError;
  const activeCount = kind === "media" ? items.length : musicItems.length;

  const posterFor = useCallback(
    (id?: string) => (serverUrl && id ? artworkUrl(serverUrl, id, "poster", 120) : undefined),
    [serverUrl],
  );

  const typeLabel = useCallback(
    (type?: string) => {
      const key = type ? ITEM_TYPE_KEYS[type] : undefined;
      return key ? t(key) : "";
    },
    [t],
  );

  const showRecents = visible && trimmed.length === 0 && recents.length > 0;

  if (!visible || (trimmed.length <= 1 && !showRecents)) return null;

  if (showRecents) {
    return (
      <View
        style={[styles.panel, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, shadowColor: theme.colors.shadow }, style]}
      >
        <View style={[styles.recentsHeader, { borderBottomColor: theme.colors.border }]}>
          <View style={styles.recentsHeaderTitle}>
            <Ionicons name="time-outline" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
              {t("media.search.recentSearches")}
            </Text>
          </View>
          <Pressable onPress={onClearRecents} accessibilityRole="button" accessibilityLabel={t("media.search.clearAll")}>
            <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("media.search.clearAll")}</Text>
          </Pressable>
        </View>
        <ScrollView keyboardShouldPersistTaps="handled">
          {recents.map((r) => (
            <View key={r} style={styles.recentRow}>
              <Pressable
                onPress={() => onPickRecent(r)}
                style={styles.recentRowLabel}
                accessible
                accessibilityRole="button"
                accessibilityLabel={r}
              >
                <Ionicons name="time-outline" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
                <Text style={{ color: theme.colors.text, fontSize: FONT_SIZE.sm, flex: 1 }} numberOfLines={1}>
                  {r}
                </Text>
              </Pressable>
              <Pressable
                onPress={() => onRemoveRecent(r)}
                accessible
                accessibilityRole="button"
                accessibilityLabel={t("common.removeItem", { item: r })}
                hitSlop={10}
              >
                <Ionicons name="close" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
              </Pressable>
            </View>
          ))}
        </ScrollView>
      </View>
    );
  }

  return (
    <View
      style={[styles.panel, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, shadowColor: theme.colors.shadow }, style]}
    >
      {/* Scope selector: video catalog (default) vs music. */}
      <View style={[styles.scopeRow, { borderBottomColor: theme.colors.border }]} accessibilityRole="tablist">
        {(["media", "music"] as const).map((k) => (
          <Pressable
            key={k}
            onPress={() => onKindChange(k)}
            style={[styles.scopeChip, { backgroundColor: kind === k ? theme.colors.accent : "transparent" }]}
            accessibilityRole="tab"
            accessibilityState={{ selected: kind === k }}
          >
            <Text style={{ color: kind === k ? theme.colors.onAccent : theme.colors.textMuted, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
              {k === "media" ? t("media.search.sectionMedia") : t("media.search.sectionMusic")}
            </Text>
          </Pressable>
        ))}
      </View>

      <ScrollView keyboardShouldPersistTaps="handled" style={styles.resultsScroll}>
        {activeLoading ? (
          <View style={styles.centerRow}>
            <ActivityIndicator color={theme.colors.accent} size="small" />
            <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("media.search.searching")}</Text>
          </View>
        ) : activeError ? (
          <LoadFailed inline message={t("media.search.searchFailed")} onRetry={() => void (kind === "media" ? refetch() : refetchMusic())} />
        ) : activeCount === 0 ? (
          <View style={styles.emptyRow}>
            <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("media.search.noResults")}</Text>
          </View>
        ) : kind === "media" ? (
          <>
            {items.map((item) => (
              <Pressable
                key={item.id}
                onPress={() => onSelectMedia(item)}
                style={styles.resultRow}
                accessible
                accessibilityRole="button"
                accessibilityLabel={displayTitle(item, t)}
              >
                <Image source={{ uri: posterFor(item.id), headers: authHeaders }} style={styles.poster} contentFit="cover" />
                <View style={{ flex: 1 }}>
                  <Text style={[styles.resultTitle, { color: theme.colors.text }]} numberOfLines={1}>
                    {displayTitle(item, t)}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
                    {typeLabel(item.type)}
                    {item.year ? ` · ${item.year}` : ""}
                  </Text>
                </View>
                <Ionicons name="chevron-forward" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
              </Pressable>
            ))}
            {totalHits > items.length && (
              <Pressable onPress={onSeeAll} style={[styles.seeAllRow, { borderTopColor: theme.colors.border }]} accessibilityRole="button">
                <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
                  {t("media.search.viewAllResults", { count: totalHits })}
                </Text>
              </Pressable>
            )}
          </>
        ) : (
          musicItems.map((tr) => {
            const cover = tr.coverArtUrl
              ? (/^https?:/i.test(tr.coverArtUrl) ? tr.coverArtUrl : `${serverUrl ?? ""}${tr.coverArtUrl}`)
              : undefined;
            return (
              <Pressable
                key={tr.id}
                onPress={() => onSelectTrack(tr)}
                style={styles.resultRow}
                accessible
                accessibilityRole="button"
                accessibilityLabel={tr.title ?? ""}
              >
                {cover ? (
                  <Image source={{ uri: cover, headers: authHeaders }} style={styles.musicCover} contentFit="cover" />
                ) : (
                  <View style={[styles.musicCover, styles.musicCoverPlaceholder, { backgroundColor: theme.colors.surface }]}>
                    <Ionicons name="musical-notes" size={ICON_SIZE.md} color={theme.colors.textMuted} />
                  </View>
                )}
                <View style={{ flex: 1 }}>
                  <Text style={[styles.resultTitle, { color: theme.colors.text }]} numberOfLines={1}>
                    {tr.title}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }} numberOfLines={1}>
                    {[tr.artist, tr.album].filter(Boolean).join(" · ")}
                  </Text>
                </View>
                <Ionicons name="play" size={ICON_SIZE.sm} color={theme.colors.accent} />
              </Pressable>
            );
          })
        )}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  panel: {
    borderRadius: 8,
    borderWidth: 1,
    overflow: "hidden",
    elevation: 8,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.15,
    shadowRadius: 8,
    maxHeight: "70%",
  },
  resultsScroll: {
    flexGrow: 0,
  },
  scopeRow: {
    flexDirection: "row",
    gap: 6,
    padding: 8,
    borderBottomWidth: 1,
  },
  scopeChip: {
    paddingHorizontal: 10,
    paddingVertical: 5,
    borderRadius: 6,
  },
  centerRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    padding: 16,
  },
  emptyRow: {
    padding: 16,
    alignItems: "center",
  },
  resultRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 12,
    paddingVertical: 8,
    gap: 10,
  },
  poster: {
    width: 32,
    height: 48,
    borderRadius: 4,
  },
  musicCover: {
    width: 36,
    height: 36,
    borderRadius: 4,
  },
  musicCoverPlaceholder: {
    alignItems: "center",
    justifyContent: "center",
  },
  resultTitle: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "500",
  },
  seeAllRow: {
    borderTopWidth: 1,
    paddingHorizontal: 12,
    paddingVertical: 10,
    alignItems: "center",
  },
  recentsHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderBottomWidth: 1,
  },
  recentsHeaderTitle: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
  },
  recentRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 12,
    paddingVertical: 8,
    gap: 10,
  },
  recentRowLabel: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
});
