import { useState, useRef, useCallback } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic } from "@evey/api";
import type { V1MediaItem, V1Track } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useDebounce } from "../hooks/useDebounce";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { useServerUrl } from "../hooks/useServerUrl";
import { useMusicQueueStore } from "../stores/musicQueue";
import { searchTrackMusicSource } from "./music/musicSource";
import { LoadFailed } from "./LoadFailed";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

export function HeaderSearch() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [focused, setFocused] = useState(false);
  const inputRef = useRef<TextInput>(null);
  const debouncedQuery = useDebounce(query, 250);
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();
  const [kind, setKind] = useState<"media" | "music">("media");
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia(
    { query: debouncedQuery, limit: 6 },
    { query: { enabled: debouncedQuery.trim().length > 1 && kind === "media" } }
  );
  const { data: musicData, isLoading: isMusicLoading, isError: isMusicError, refetch: refetchMusic } = useMusicServiceSearchMusic(
    { query: debouncedQuery, limit: 6 },
    { query: { enabled: debouncedQuery.trim().length > 1 && kind === "music" } }
  );

  const results = data?.items ?? [];
  const musicResults = musicData?.tracks ?? [];
  const showDropdown = focused && debouncedQuery.trim().length > 1;

  const handleSelect = useCallback((item: V1MediaItem) => {
    setQuery("");
    setFocused(false);
    inputRef.current?.blur();
    router.push(`/media/${item.id}`);
  }, []);

  const handleSelectTrack = useCallback((tr: V1Track) => {
    setFocused(false);
    inputRef.current?.blur();
    setQueue([{
      id: tr.id ?? "",
      title: tr.title ?? "",
      artist: tr.artist ?? "",
      album: tr.album ?? "",
      duration: tr.duration ?? 0,
    }], 0, searchTrackMusicSource(tr.albumId, tr.album));
  }, [setQueue]);

  const handleSubmit = useCallback(() => {
    if (!query.trim()) return;
    setFocused(false);
    inputRef.current?.blur();
    router.push({
      pathname: "/(auth)/(tabs)/search",
      params: { q: query.trim(), ...(kind === "music" ? { kind: "music" } : {}) },
    });
  }, [query, kind]);

  return (
    <View style={styles.wrapper}>
      <View style={[styles.inputRow, { backgroundColor: theme.colors.surfaceRaised, borderColor: focused ? theme.colors.accent : theme.colors.border }]}>
        <Ionicons name="search-outline" size={ICON_SIZE.md} color={theme.colors.textMuted} style={{ marginRight: 6 }} />
        <TextInput
          ref={inputRef}
          value={query}
          onChangeText={setQuery}
          onFocus={() => setFocused(true)}
          onBlur={() => setTimeout(() => setFocused(false), 150)}
          onSubmitEditing={handleSubmit}
          placeholder={t("media.search.placeholder", "Search movies, series…")}
          placeholderTextColor={theme.colors.textMuted}
          returnKeyType="search"
          autoCapitalize="none"
          autoCorrect={false}
          style={[styles.input, { color: theme.colors.text }]}
        />
        {query.length > 0 && (
          <Pressable onPress={() => { setQuery(""); inputRef.current?.focus(); }} accessible accessibilityLabel={t("common.clear", "Clear")} accessibilityRole="button">
            <Ionicons name="close-circle" size={ICON_SIZE.md} color={theme.colors.textMuted} />
          </Pressable>
        )}
      </View>

      {showDropdown && (
        <View style={[styles.dropdown, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, shadowColor: theme.colors.shadow }]}>
          {/* Scope selector: video catalog (default) vs music */}
          <View style={[styles.scopeRow, { borderBottomColor: theme.colors.border }]}>
            {(["media", "music"] as const).map((k) => (
              <Pressable
                key={k}
                onPress={() => setKind(k)}
                style={[styles.scopeChip, { backgroundColor: kind === k ? theme.colors.accent : "transparent" }]}
                accessibilityRole="button"
              >
                <Text style={{ color: kind === k ? theme.colors.onAccent : theme.colors.textMuted, fontSize: FONT_SIZE.xs, fontWeight: "600" }}>
                  {k === "media" ? t("media.search.sectionMedia") : t("media.search.sectionMusic")}
                </Text>
              </Pressable>
            ))}
          </View>

          {kind === "music" ? (
            isMusicLoading ? (
              <View style={styles.emptyRow}>
                <ActivityIndicator color={theme.colors.accent} size="small" />
              </View>
            ) : isMusicError ? (
              <LoadFailed inline onRetry={() => void refetchMusic()} />
            ) : musicResults.length === 0 ? (
              <View style={styles.emptyRow}>
                <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("media.search.noResults", "No results")}</Text>
              </View>
            ) : (
              <>
                {musicResults.map((tr) => {
                  const cover = tr.coverArtUrl
                    ? (/^https?:/i.test(tr.coverArtUrl) ? tr.coverArtUrl : `${serverUrl ?? ""}${tr.coverArtUrl}`)
                    : undefined;
                  return (
                    <Pressable key={tr.id} onPress={() => handleSelectTrack(tr)} style={styles.resultRow} accessible accessibilityLabel={tr.title ?? ""} accessibilityRole="button">
                      {cover ? (
                        <Image source={{ uri: cover, headers: authHeaders }} style={styles.musicCover} contentFit="cover" />
                      ) : (
                        <View style={[styles.musicCover, { alignItems: "center", justifyContent: "center", backgroundColor: theme.colors.surface }]}>
                          <Ionicons name="musical-notes" size={ICON_SIZE.md} color={theme.colors.textMuted} />
                        </View>
                      )}
                      <View style={{ flex: 1 }}>
                        <Text style={[styles.resultTitle, { color: theme.colors.text }]} numberOfLines={1}>{tr.title}</Text>
                        <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }} numberOfLines={1}>
                          {[tr.artist, tr.album].filter(Boolean).join(" · ")}
                        </Text>
                      </View>
                      <Ionicons name="play" size={ICON_SIZE.sm} color={theme.colors.accent} />
                    </Pressable>
                  );
                })}
                <Pressable onPress={handleSubmit} style={[styles.seeAllRow, { borderTopColor: theme.colors.border }]}>
                  <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.xs, fontWeight: "500" }}>
                    {t("common.seeAll", "See all results for")} "{query}"
                  </Text>
                </Pressable>
              </>
            )
          ) : isLoading ? (
            <View style={styles.emptyRow}>
              <ActivityIndicator color={theme.colors.accent} size="small" />
            </View>
          ) : isError ? (
            <LoadFailed inline onRetry={() => void refetch()} />
          ) : results.length === 0 ? (
            <View style={styles.emptyRow}>
              <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("media.search.noResults", "No results")}</Text>
            </View>
          ) : (
            <>
              {results.map((item) => {
                const posterUrl = `${serverUrl}/api/v1/media/${item.id}/artwork/poster?w=120`;
                return (
                  <Pressable key={item.id} onPress={() => handleSelect(item)} style={styles.resultRow} accessible accessibilityLabel={item.title ?? ""} accessibilityRole="button">
                    <Image
                      source={{ uri: posterUrl, headers: authHeaders }}
                      style={styles.poster}
                      contentFit="cover"
                    />
                    <View style={{ flex: 1 }}>
                      <Text style={[styles.resultTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.title}</Text>
                      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
                        {item.type?.replace("MEDIA_ITEM_TYPE_", "").toLowerCase()}
                        {item.year ? ` · ${item.year}` : ""}
                      </Text>
                    </View>
                    <Ionicons name="chevron-forward" size={ICON_SIZE.sm} color={theme.colors.textMuted} />
                  </Pressable>
                );
              })}
              <Pressable onPress={handleSubmit} style={[styles.seeAllRow, { borderTopColor: theme.colors.border }]}>
                <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.xs, fontWeight: "500" }}>
                  {t("common.seeAll", "See all results for")} "{query}"
                </Text>
              </Pressable>
            </>
          )}
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: {
    flex: 1,
    position: "relative",
    zIndex: 100,
  },
  inputRow: {
    flexDirection: "row",
    alignItems: "center",
    borderRadius: 8,
    borderWidth: 1,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  input: {
    flex: 1,
    fontSize: FONT_SIZE.sm,
    padding: 0,
    margin: 0,
  },
  dropdown: {
    position: "absolute",
    top: "100%",
    left: -60,
    right: -80,
    marginTop: 4,
    borderRadius: 8,
    borderWidth: 1,
    overflow: "hidden",
    elevation: 8,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.15,
    shadowRadius: 8,
    zIndex: 999,
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
  musicCover: {
    width: 36,
    height: 36,
    borderRadius: 4,
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
  resultTitle: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "500",
  },
  seeAllRow: {
    borderTopWidth: 1,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
});
