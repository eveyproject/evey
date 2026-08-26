import { useState, useCallback } from "react";
import { displayTitle } from "@evey/api";
import { View, Text, TextInput, FlatList, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic } from "@evey/api";
import type { V1MediaItem, V1Track } from "@evey/api";
import { startVoiceSearch } from "@evey/voice-search";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";
import { mediaTypeLabelKey } from "../../../src/lib/mediaTypeLabel";
import { artworkUrl } from "../../../src/lib/artwork";

function tvSearchKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

function tvTrackKeyExtractor(item: V1Track) {
  return item.id ?? "";
}

function TVSearchSeparator() {
  return <View style={{ height: 12 }} />;
}

// Mirrors the local coverUri() helper in music.tsx / music/album/[id].tsx /
// music/artist/[id].tsx: a track's coverArtUrl already comes back
// server-relative, so it needs the active server prefixed unless it is
// already absolute.
function coverUri(serverUrl: string, coverArtUrl?: string): string | undefined {
  if (!coverArtUrl) return undefined;
  return /^https?:/i.test(coverArtUrl) ? coverArtUrl : `${serverUrl}${coverArtUrl}`;
}

// The screen shows at most one results view at a time (error, loading,
// prompt, empty, or a single results list) - each with its own first-item
// hasTVPreferredFocus. Making the choice an explicit, testable function
// (instead of an inline JSX ternary) proves by construction that exactly
// one view - and so exactly one initial-focus claimant - is ever rendered.
export type SearchResultsView =
  | "error"
  | "loading"
  | "prompt"
  | "musicEmpty"
  | "musicResults"
  | "mediaEmpty"
  | "mediaResults";

export function selectSearchResultsView(params: {
  searchKind: "media" | "music";
  isError: boolean;
  activeLoading: boolean;
  hasQuery: boolean;
  musicItemsCount: number;
  mediaItemsCount: number;
}): SearchResultsView {
  const { searchKind, isError, activeLoading, hasQuery, musicItemsCount, mediaItemsCount } = params;
  // isError is the ACTIVE side's error - a failed music search must show the
  // error state too, not pose as "no results".
  if (isError && hasQuery) return "error";
  if (activeLoading && hasQuery) return "loading";
  if (!hasQuery) return "prompt";
  if (searchKind === "music") return musicItemsCount === 0 ? "musicEmpty" : "musicResults";
  return mediaItemsCount === 0 ? "mediaEmpty" : "mediaResults";
}

// Typing a title on a d-pad-driven virtual keyboard is the worst input a
// remote has to offer - voice is the natural one here. Recognized text just
// fills the same query state the keyboard does; it never bypasses or
// reimplements the search itself. "unavailable" (no recognizer, no native
// module, permission declined further up the OS) and "empty" (cancelled, or
// nothing understood) are both deliberately silent: the keyboard next to
// this button is always the fallback and must never be interrupted by it.
function VoiceSearchButton({ onRecognized }: { onRecognized: (text: string) => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [listening, setListening] = useState(false);

  const handlePress = useCallback(async () => {
    if (listening) return;
    setListening(true);
    try {
      const result = await startVoiceSearch();
      if (result.status === "recognized") {
        onRecognized(result.text);
      }
    } finally {
      setListening(false);
    }
  }, [listening, onRecognized]);

  return (
    <FocusableCard
      onPress={handlePress}
      disabled={listening}
      accessibilityLabel={t(listening ? "media.search.voiceSearchListening" : "media.search.voiceSearch")}
      style={[styles.voiceButton, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md }]}
    >
      {listening ? (
        <ActivityIndicator color={theme.colors.onAccent} />
      ) : (
        <Ionicons name="mic" size={28} color={theme.colors.onAccent} />
      )}
    </FocusableCard>
  );
}

function ScopeTab({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }) {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      accessibilityRole="button"
      accessibilityLabel={label}
      style={{
        paddingHorizontal: 20,
        paddingVertical: 10,
        marginRight: 12,
        borderRadius: 8,
        borderWidth: 3,
        borderColor: focused ? theme.colors.accent : "transparent",
        backgroundColor: active ? theme.colors.accent : theme.colors.surface,
      }}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 18, fontWeight: "600" }}>{label}</Text>
    </Pressable>
  );
}

export default function SearchScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [query, setQuery] = useState("");
  const [inputFocused, setInputFocused] = useState(false);
  const [searchKind, setSearchKind] = useState<"media" | "music">("media");
  // Trim then gate at > 1 char, like web and mobile: a single character fired
  // a request per keystroke here for results nobody can use.
  const debouncedQuery = useDebounce(query.trim(), 300);
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 1 && searchKind === "media" } },
  );
  const { data: musicData, isLoading: musicLoading, isError: musicError, refetch: musicRefetch } = useMusicServiceSearchMusic(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 1 && searchKind === "music" } },
  );

  const items = data?.items ?? [];
  const musicItems = musicData?.tracks ?? [];
  const activeLoading = searchKind === "media" ? isLoading : musicLoading;
  const activeRefetch = searchKind === "media" ? refetch : musicRefetch;
  const resultsView = selectSearchResultsView({
    searchKind,
    isError: searchKind === "media" ? isError : musicError,
    activeLoading,
    hasQuery: debouncedQuery.length > 1,
    musicItemsCount: musicItems.length,
    mediaItemsCount: items.length,
  });

  // Translated, never the raw enum: "2024 · Film", not "2024 · movie".
  const typeLabel = useCallback(
    (type?: string) => {
      const key = mediaTypeLabelKey(type);
      return key ? t(key) : "";
    },
    [t],
  );

  const renderMusicItem = useCallback(({ item, index }: { item: V1Track; index: number }) => {
    const uri = coverUri(serverUrl, item.coverArtUrl);
    return (
      <FocusableCard
        onPress={() => setQueue([{
          id: item.id ?? "",
          title: item.title ?? "",
          artist: item.artist ?? "",
          album: item.album ?? "",
          duration: item.duration ?? 0,
        }], 0)}
        style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        hasTVPreferredFocus={index === 0}
        accessibilityLabel={item.title}
      >
        <View style={[styles.musicThumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface, overflow: "hidden" }]}>
          {uri ? (
            <Image
              source={{ uri, headers: authHeaders }}
              style={{ width: 72, height: 72 }}
              contentFit="cover"
              transition={150}
              recyclingKey={item.id}
            />
          ) : (
            <Ionicons name="musical-notes" size={24} color={theme.colors.textMuted} />
          )}
        </View>
        <View style={styles.info}>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
            {item.title}
          </Text>
          <Text style={[styles.meta, { color: theme.colors.textMuted }]} numberOfLines={1}>
            {[item.artist, item.album].filter(Boolean).join(" · ")}
          </Text>
        </View>
        <Ionicons name="play" size={24} color={theme.colors.accent} />
      </FocusableCard>
    );
  }, [theme, setQueue, serverUrl, authHeaders]);

  const renderItem = useCallback(({ item, index }: { item: V1MediaItem; index: number }) => {
    const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", 160) : undefined;
    const genres = (item.genres ?? []).filter(Boolean).slice(0, 2);
    const label = typeLabel(item.type);
    return (
      <FocusableCard
        onPress={() => router.push(`/media/${item.id}`)}
        style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        hasTVPreferredFocus={index === 0}
        accessibilityLabel={displayTitle(item, t)}
      >
        <View style={[styles.thumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface, overflow: "hidden" }]}>
          {posterUri ? (
            <Image
              source={{ uri: posterUri, headers: authHeaders }}
              style={{ width: 80, height: 120 }}
              contentFit="cover"
              transition={150}
              recyclingKey={item.id}
            />
          ) : (
            <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>?</Text>
          )}
        </View>
        <View style={styles.info}>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
            {displayTitle(item, t)}
          </Text>
          <View style={styles.metaRow}>
            {item.year ? (
              <Text style={[styles.metaText, { color: theme.colors.textMuted }]}>{item.year}</Text>
            ) : null}
            {label ? (
              <View style={[styles.badge, { backgroundColor: theme.colors.bg }]}>
                <Text style={[styles.badgeText, { color: theme.colors.textMuted }]}>{label}</Text>
              </View>
            ) : null}
            {genres.length > 0 ? (
              <Text style={[styles.metaText, { color: theme.colors.textMuted }]} numberOfLines={1}>
                {genres.join(", ")}
              </Text>
            ) : null}
          </View>
        </View>
      </FocusableCard>
    );
  }, [theme, router, typeLabel, t, serverUrl, authHeaders]);

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <View style={styles.searchRow}>
        <TextInput
          style={[
            styles.input,
            {
              backgroundColor: theme.colors.surface,
              color: theme.colors.text,
              borderRadius: theme.radii.md,
              borderColor: inputFocused ? theme.colors.accent : theme.colors.border,
            },
          ]}
          placeholder={t("media.search.placeholder")}
          placeholderTextColor={theme.colors.textMuted}
          value={query}
          onChangeText={setQuery}
          autoCapitalize="none"
          autoCorrect={false}
          returnKeyType="search"
          onFocus={() => setInputFocused(true)}
          onBlur={() => setInputFocused(false)}
          accessible={true}
          accessibilityLabel={t("media.search.placeholder")}
        />
        <VoiceSearchButton onRecognized={setQuery} />
      </View>

      {/* Scope selector: video catalog (default) vs music */}
      <View style={{ flexDirection: "row", paddingHorizontal: 24, marginBottom: 8 }}>
        <ScopeTab label={t("media.search.sectionMedia")} active={searchKind === "media"} onPress={() => setSearchKind("media")} />
        <ScopeTab label={t("media.search.sectionMusic")} active={searchKind === "music"} onPress={() => setSearchKind("music")} />
      </View>

      {/* Results panel: floating card anchored below the input/tabs, mirroring
          the web dropdown - a raised surface distinct from the screen
          background rather than a flat, full-page list. */}
      <View style={styles.resultsAnchor}>
        <View
          style={[
            styles.resultsPanel,
            {
              backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface,
              borderRadius: theme.radii.lg ?? theme.radii.md,
              shadowColor: theme.colors.shadow ?? "#000",
            },
          ]}
        >
          <View style={[styles.resultsPanelInner, { borderRadius: theme.radii.lg ?? theme.radii.md }]}>
            {resultsView === "error" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 12 }}>{t("common.loadError")}</Text>
                <FocusableCard onPress={() => activeRefetch()} hasTVPreferredFocus accessibilityLabel={t("common.retry")}>
                  <Text style={{ color: theme.colors.accent, fontSize: 20 }}>{t("common.retry")}</Text>
                </FocusableCard>
              </View>
            ) : resultsView === "loading" ? (
              <View style={styles.center}>
                <ActivityIndicator color={theme.colors.accent} size="large" />
              </View>
            ) : resultsView === "prompt" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>
                  {t("media.search.searchLibrary")}
                </Text>
              </View>
            ) : resultsView === "musicEmpty" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.search.noResults")}</Text>
              </View>
            ) : resultsView === "musicResults" ? (
              <FlatList
                data={musicItems}
                keyExtractor={tvTrackKeyExtractor}
                renderItem={renderMusicItem}
                style={styles.resultsList}
                contentContainerStyle={{ padding: 24 }}
                ItemSeparatorComponent={TVSearchSeparator}
                initialNumToRender={6}
                maxToRenderPerBatch={10}
                windowSize={5}
                removeClippedSubviews={true}
              />
            ) : resultsView === "mediaEmpty" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.search.noResults")}</Text>
              </View>
            ) : (
              <FlatList
                data={items}
                keyExtractor={tvSearchKeyExtractor}
                renderItem={renderItem}
                style={styles.resultsList}
                contentContainerStyle={{ padding: 24 }}
                ItemSeparatorComponent={TVSearchSeparator}
                initialNumToRender={6}
                maxToRenderPerBatch={10}
                windowSize={5}
                removeClippedSubviews={true}
              />
            )}
          </View>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  searchRow: {
    flexDirection: "row",
    alignItems: "center",
    marginHorizontal: 24,
    marginTop: 32,
    marginBottom: 16,
  },
  input: {
    flex: 1,
    paddingHorizontal: 20,
    paddingVertical: 16,
    fontSize: 22,
    borderWidth: 3,
  },
  // Generous focus target on purpose - a TV remote is used at sofa distance,
  // not with a mouse pointer, so this matches the thumbnails' footprint
  // rather than a phone-sized icon button.
  voiceButton: {
    width: 72,
    height: 72,
    marginLeft: 16,
    justifyContent: "center",
    alignItems: "center",
  },
  // The anchor gives the panel its floating inset (matching the search row's
  // own horizontal margin) and a bottom gap so the screen background stays
  // visible beneath it, instead of the panel stretching edge to edge like the
  // old flat list did.
  resultsAnchor: {
    flex: 1,
    marginHorizontal: 24,
    marginBottom: 24,
  },
  // Elevation/shadow lives on this outer view (no overflow clipping here) so
  // the rounded corners of resultsPanelInner don't cut the shadow off.
  resultsPanel: {
    flex: 1,
    elevation: 8,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 12,
  },
  resultsPanelInner: {
    flex: 1,
    overflow: "hidden",
  },
  resultsList: {
    flex: 1,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    padding: 16,
  },
  thumb: {
    width: 80,
    height: 120,
    marginRight: 20,
    justifyContent: "center",
    alignItems: "center",
  },
  musicThumb: {
    width: 72,
    height: 72,
    marginRight: 20,
    justifyContent: "center",
    alignItems: "center",
  },
  info: { flex: 1 },
  title: { fontSize: 22, fontWeight: "600" },
  meta: { fontSize: 16, marginTop: 4 },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    flexWrap: "wrap",
    gap: 10,
    marginTop: 4,
  },
  metaText: { fontSize: 16 },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 6,
  },
  badgeText: { fontSize: 14, fontWeight: "600" },
});
