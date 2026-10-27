import { useState, useCallback } from "react";
import { displayTitle } from "@evey/api";
import { View, Text, TextInput, FlatList, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic } from "@evey/api";
import type { V1MediaItem, V1Track } from "@evey/api";
import { startVoiceSearch } from "@evey/voice-search";
import { useTheme } from "../providers/ThemeProvider";
import { useDebounce } from "../hooks/useDebounce";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { FocusableCard } from "./FocusableCard";
import { useMusicQueueStore } from "../stores/musicQueue";
import { mediaTypeLabelKey } from "../lib/mediaTypeLabel";
import { artworkUrl, coverArtUri } from "../lib/artwork";

const MUSIC_THUMB_SIZE = 72;

function tvSearchKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

function tvTrackKeyExtractor(item: V1Track) {
  return item.id ?? "";
}

function TVSearchSeparator() {
  return <View style={{ height: 12 }} />;
}

// Uses the shared coverArtUri() helper (src/lib/artwork.ts), like music.tsx / music/album/[id].tsx /
// music/artist/[id].tsx: a track's coverArtUrl already comes back
// server-relative, so it needs the active server prefixed unless it is
// already absolute.
// One panel, two hosts: the /search route and the header's dropdown overlay
// (SearchOverlay). Extracted from the route on 2026-09-19 so the dropdown is
// the SAME search - same query, same debounce, same rows - and not a second
// implementation drifting from it.
//
// The panel shows at most one results view at a time (error, loading,
// prompt, empty, or a single results list) - each with its own first-item
// hasTVPreferredFocus. Making the choice an explicit, testable function
// (instead of an inline JSX ternary) proves by construction that exactly
// one view - and so exactly one initial-focus claimant - is ever rendered.
import { selectSearchResultsView, type SearchResultsView } from "@evey/client";
import { useMediaPrefetch } from "../hooks/useMediaPrefetch";

export { selectSearchResultsView };
export type { SearchResultsView };

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
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 17, fontWeight: "600" }}>{label}</Text>
    </Pressable>
  );
}

export interface SearchPanelProps {
  // The overlay opens on the field: the input must hold the remote from the
  // first frame, or the viewer types into nothing. A screen mounting the panel
  // leaves it false and lets its own layout decide.
  hasTVPreferredFocus?: boolean;
  // Called once the panel has sent the viewer somewhere (a media page, a
  // track): the overlay closes behind them, a screen ignores it.
  onNavigate?: () => void;
}

export function SearchPanel({ hasTVPreferredFocus = false, onNavigate }: SearchPanelProps) {
  const { prefetchOnFocus, cancelPrefetch } = useMediaPrefetch();
  const theme = useTheme();
  const router = useRouter();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [query, setQuery] = useState("");
  // While the field has the remote, the results must NOT claim it: the first
  // row's preferred focus fired on every keystroke (a new query mounts a new
  // first row), the focus left the field, the on-screen keyboard closed, and
  // the viewer typed one letter at a time (owner, 2026-09-22). The rows stay
  // focusable - "down" from the field reaches them through the focus engine.
  const [inputFocused, setInputFocused] = useState(false);
  const [searchKind, setSearchKind] = useState<"media" | "music">("media");
  // The results box is measured, not flexed. Inside the header's Modal the
  // flex chain under this point never resolved on the box (2026-09-19): the
  // list came out 2 px tall with its rows correctly built behind it, so the
  // dropdown looked empty. One definite height, taken from the slot the layout
  // actually gave us, and everything below it resolves in both hosts.
  const [resultsHeight, setResultsHeight] = useState(0);
  // flex:1 would win over any height added next to it, so the three nested
  // boxes take one or the other, never both.
  const resultsBox = resultsHeight > 0 ? { height: resultsHeight } : { flex: 1 };
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
    const uri = coverArtUri(serverUrl, item.coverArtUrl, MUSIC_THUMB_SIZE * 2);
    return (
      <FocusableCard
        onPress={() => {
          onNavigate?.();
          setQueue([{
            id: item.id ?? "",
            title: item.title ?? "",
            artist: item.artist ?? "",
            album: item.album ?? "",
            duration: item.duration ?? 0,
          }], 0);
        }}
        style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        hasTVPreferredFocus={index === 0 && !inputFocused}
        accessibilityLabel={item.title}
      >
        <View style={[styles.musicThumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface, overflow: "hidden" }]}>
          {uri ? (
            <Image
              cachePolicy="memory-disk"
              source={{ uri, headers: authHeaders }}
              style={{ width: 72, height: 72 }}
              contentFit="cover"
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
  }, [theme, setQueue, serverUrl, authHeaders, onNavigate, inputFocused]);

  const renderItem = useCallback(({ item, index }: { item: V1MediaItem; index: number }) => {
    const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", 160, item) : undefined;
    const genres = (item.genres ?? []).filter(Boolean).slice(0, 2);
    const label = typeLabel(item.type);
    return (
      <FocusableCard
        onPress={() => {
          onNavigate?.();
          router.push(`/media/${item.id}`);
        }}
        onFocus={() => prefetchOnFocus(item.id)}
        onBlur={cancelPrefetch}
        style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        hasTVPreferredFocus={index === 0 && !inputFocused}
        accessibilityLabel={displayTitle(item, t)}
      >
        <View style={[styles.thumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface, overflow: "hidden" }]}>
          {posterUri ? (
            <Image
              cachePolicy="memory-disk"
              source={{ uri: posterUri, headers: authHeaders }}
              style={{ width: 80, height: 120 }}
              contentFit="cover"
              recyclingKey={item.id}
            />
          ) : (
            <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>?</Text>
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
  }, [theme, router, typeLabel, t, serverUrl, authHeaders, onNavigate, inputFocused, prefetchOnFocus, cancelPrefetch]);

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
          hasTVPreferredFocus={hasTVPreferredFocus}
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
      <View
        style={styles.resultsAnchor}
        onLayout={(e) => setResultsHeight(e.nativeEvent.layout.height)}
      >
        <View
          style={[
            styles.resultsPanel,
            resultsBox,
            {
              backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface,
              borderRadius: theme.radii.lg ?? theme.radii.md,
              shadowColor: theme.colors.shadow ?? "#000",
            },
          ]}
        >
          <View
            style={[
              styles.resultsPanelInner,
              resultsBox,
              { borderRadius: theme.radii.lg ?? theme.radii.md },
            ]}
          >
            {resultsView === "error" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 12 }}>{t("common.loadError")}</Text>
                <FocusableCard onPress={() => activeRefetch()} hasTVPreferredFocus accessibilityLabel={t("common.retry")}>
                  <Text style={{ color: theme.colors.accent, fontSize: 18 }}>{t("common.retry")}</Text>
                </FocusableCard>
              </View>
            ) : resultsView === "loading" ? (
              <View style={styles.center}>
                <ActivityIndicator color={theme.colors.accent} size="large" />
              </View>
            ) : resultsView === "prompt" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>
                  {t("media.search.searchLibrary")}
                </Text>
              </View>
            ) : resultsView === "musicEmpty" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("media.search.noResults")}</Text>
              </View>
            ) : resultsView === "musicResults" ? (
              <FlatList
                data={musicItems}
                keyExtractor={tvTrackKeyExtractor}
                renderItem={renderMusicItem}
                style={[styles.resultsList, resultsBox]}
                contentContainerStyle={{ padding: 24 }}
                ItemSeparatorComponent={TVSearchSeparator}
                initialNumToRender={6}
                maxToRenderPerBatch={10}
                windowSize={5}
                removeClippedSubviews={true}
              />
            ) : resultsView === "mediaEmpty" ? (
              <View style={styles.center}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>{t("media.search.noResults")}</Text>
              </View>
            ) : (
              <FlatList
                data={items}
                keyExtractor={tvSearchKeyExtractor}
                renderItem={renderItem}
                style={[styles.resultsList, resultsBox]}
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
    paddingVertical: 11,
    fontSize: 20,
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
    elevation: 8,
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 12,
  },
  resultsPanelInner: {
    overflow: "hidden",
  },
  resultsList: {},
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
    width: MUSIC_THUMB_SIZE,
    height: MUSIC_THUMB_SIZE,
    marginRight: 20,
    justifyContent: "center",
    alignItems: "center",
  },
  info: { flex: 1 },
  title: { fontSize: 20, fontWeight: "600" },
  meta: { fontSize: 15, marginTop: 4 },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    flexWrap: "wrap",
    gap: 10,
    marginTop: 4,
  },
  metaText: { fontSize: 15 },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 6,
  },
  badgeText: { fontSize: 13, fontWeight: "600" },
});
