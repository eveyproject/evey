import { useState, useCallback } from "react";
import { displayTitle } from "@evey/api";
import { View, Text, TextInput, FlatList, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic } from "@evey/api";
import type { V1MediaItem, V1Track } from "@evey/api";
import { startVoiceSearch } from "@evey/voice-search";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";
import { mediaTypeLabelKey } from "../../../src/lib/mediaTypeLabel";



function tvSearchKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

function tvTrackKeyExtractor(item: V1Track) {
  return item.id ?? "";
}

function TVSearchSeparator() {
  return <View style={{ height: 12 }} />;
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
  const [query, setQuery] = useState("");
  const [inputFocused, setInputFocused] = useState(false);
  const [searchKind, setSearchKind] = useState<"media" | "music">("media");
  const debouncedQuery = useDebounce(query, 300);
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const { data, isLoading, isError, refetch } = useLibraryServiceSearchMedia(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 0 && searchKind === "media" } },
  );
  const { data: musicData, isLoading: musicLoading, isError: musicError, refetch: musicRefetch } = useMusicServiceSearchMusic(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 0 && searchKind === "music" } },
  );

  const items = data?.items ?? [];
  const musicItems = musicData?.tracks ?? [];
  const activeLoading = searchKind === "media" ? isLoading : musicLoading;
  const activeRefetch = searchKind === "media" ? refetch : musicRefetch;
  const resultsView = selectSearchResultsView({
    searchKind,
    isError: searchKind === "media" ? isError : musicError,
    activeLoading,
    hasQuery: debouncedQuery.length > 0,
    musicItemsCount: musicItems.length,
    mediaItemsCount: items.length,
  });

  const renderMusicItem = useCallback(({ item, index }: { item: V1Track; index: number }) => (
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
      <View style={[styles.musicThumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface }]}>
        <Ionicons name="musical-notes" size={24} color={theme.colors.textMuted} />
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
  ), [theme, setQueue]);

  // Translated, never the raw enum: "2024 · Film", not "2024 · movie".
  const typeLabel = useCallback(
    (type?: string) => {
      const key = mediaTypeLabelKey(type);
      return key ? t(key) : "";
    },
    [t],
  );

  const renderItem = useCallback(({ item, index }: { item: V1MediaItem; index: number }) => (
    <FocusableCard
      onPress={() => router.push(`/media/${item.id}`)}
      style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      hasTVPreferredFocus={index === 0}
      accessibilityLabel={displayTitle(item)}
    >
      <View style={[styles.thumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>?</Text>
      </View>
      <View style={styles.info}>
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
          {displayTitle(item)}
        </Text>
        <Text style={[styles.meta, { color: theme.colors.textMuted }]}>
          {[item.year, typeLabel(item.type)].filter(Boolean).join(" \u00B7 ")}
        </Text>
      </View>
    </FocusableCard>
  ), [theme, router, typeLabel]);

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
          contentContainerStyle={{ padding: 24 }}
          ItemSeparatorComponent={TVSearchSeparator}
          initialNumToRender={6}
          maxToRenderPerBatch={10}
          windowSize={5}
          removeClippedSubviews={true}
        />
      )}
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
});
