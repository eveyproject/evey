import { useState, useCallback } from "react";
import { View, Text, TextInput, FlatList, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia, useMusicServiceSearchMusic } from "@evey/api";
import type { V1MediaItem, V1Track } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { useMusicQueueStore } from "../../../src/stores/musicQueue";

function formatType(type?: string): string {
  if (!type) return "";
  return type.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();
}

function tvSearchKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

function tvTrackKeyExtractor(item: V1Track) {
  return item.id ?? "";
}

function TVSearchSeparator() {
  return <View style={{ height: 12 }} />;
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
  const { data: musicData, isLoading: musicLoading } = useMusicServiceSearchMusic(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 0 && searchKind === "music" } },
  );

  const items = data?.items ?? [];
  const musicItems = musicData?.tracks ?? [];
  const activeLoading = searchKind === "media" ? isLoading : musicLoading;

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

  const renderItem = useCallback(({ item, index }: { item: V1MediaItem; index: number }) => (
    <FocusableCard
      onPress={() => router.push(`/media/${item.id}`)}
      style={[styles.row, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      hasTVPreferredFocus={index === 0}
    >
      <View style={[styles.thumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised ?? theme.colors.surface }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 18 }}>?</Text>
      </View>
      <View style={styles.info}>
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
          {item.title}
        </Text>
        <Text style={[styles.meta, { color: theme.colors.textMuted }]}>
          {[item.year, formatType(item.type)].filter(Boolean).join(" \u00B7 ")}
        </Text>
      </View>
    </FocusableCard>
  ), [theme, router]);

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
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
        accessibilityLabel="Search media"
      />

      {/* Scope selector: video catalog (default) vs music */}
      <View style={{ flexDirection: "row", paddingHorizontal: 24, marginBottom: 8 }}>
        <ScopeTab label={t("media.search.sectionMedia")} active={searchKind === "media"} onPress={() => setSearchKind("media")} />
        <ScopeTab label={t("media.search.sectionMusic")} active={searchKind === "music"} onPress={() => setSearchKind("music")} />
      </View>

      {searchKind === "media" && isError && debouncedQuery.length > 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 12 }}>{t("common.loadError")}</Text>
          <Pressable onPress={() => refetch()}>
            <Text style={{ color: theme.colors.accent, fontSize: 20 }}>{t("common.retry")}</Text>
          </Pressable>
        </View>
      ) : activeLoading && debouncedQuery.length > 0 ? (
        <View style={styles.center}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : debouncedQuery.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>
            {t("media.search.searchLibrary")}
          </Text>
        </View>
      ) : searchKind === "music" ? (
        musicItems.length === 0 ? (
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.search.noResults")}</Text>
          </View>
        ) : (
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
        )
      ) : items.length === 0 ? (
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
  input: {
    marginHorizontal: 24,
    marginTop: 32,
    marginBottom: 16,
    paddingHorizontal: 20,
    paddingVertical: 16,
    fontSize: 22,
    borderWidth: 3,
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
