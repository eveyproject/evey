import { useState, useRef, useCallback } from "react";
import { View, TextInput, Pressable, Modal, StyleSheet } from "react-native";
import { router } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import * as SecureStore from "expo-secure-store";
import type { V1MediaItem, V1Track } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useDebounce } from "../hooks/useDebounce";
import { useMusicQueueStore } from "../stores/musicQueue";
import { searchTrackMusicSource } from "./music/musicSource";
import { SearchDropdown } from "./SearchDropdown";
import { MAX_SEARCH_HISTORY, pushSearchHistory, removeSearchHistoryEntry } from "./library/searchHistory";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// The header title lives inside the NATIVE toolbar, and both Android and iOS
// clip a toolbar's children to the bar's bounds - a result panel positioned
// with `position: absolute` under the field never painted on device. The
// panel is therefore rendered in a transparent `Modal`, which is its own
// native surface (not a toolbar descendant) and can't be clipped by it. The
// panel's screen position is computed by measuring this component's own
// wrapper in window coordinates on focus, so it lands directly under the
// input regardless of where the header sits.
//
// Shares its recent-searches storage (SecureStore key below) with the full
// search screen ((tabs)/search.tsx) so a query saved from either surface
// shows up in both.
const HISTORY_KEY = "evey-search-history";

function loadSearchHistory(): string[] {
  try {
    const raw = SecureStore.getItem(HISTORY_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveSearchHistory(history: string[]) {
  try {
    SecureStore.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_SEARCH_HISTORY)));
  } catch (err) {
    console.warn("caught", err);
  }
}

interface PanelLayout {
  x: number;
  y: number;
  width: number;
  height: number;
}

export function HeaderSearch() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [focused, setFocused] = useState(false);
  const [kind, setKind] = useState<"media" | "music">("media");
  const [layout, setLayout] = useState<PanelLayout | null>(null);
  const [recents, setRecents] = useState<string[]>(() => loadSearchHistory());
  const wrapperRef = useRef<View>(null);
  const inputRef = useRef<TextInput>(null);
  const debouncedQuery = useDebounce(query.trim(), 250);
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const measure = useCallback(() => {
    wrapperRef.current?.measureInWindow((x, y, width, height) => {
      setLayout({ x, y, width, height });
    });
  }, []);

  const closeDropdown = useCallback(() => {
    setFocused(false);
  }, []);

  const addToHistory = useCallback((q: string) => {
    const trimmed = q.trim();
    if (!trimmed) return;
    setRecents((prev) => {
      const updated = pushSearchHistory(prev, trimmed);
      saveSearchHistory(updated);
      return updated;
    });
  }, []);

  const removeRecent = useCallback((entry: string) => {
    setRecents((prev) => {
      const updated = removeSearchHistoryEntry(prev, entry);
      saveSearchHistory(updated);
      return updated;
    });
  }, []);

  const clearRecents = useCallback(() => {
    setRecents([]);
    saveSearchHistory([]);
  }, []);

  const navigateToFullSearch = useCallback((q: string, searchKind: "media" | "music") => {
    const trimmed = q.trim();
    addToHistory(trimmed);
    setQuery("");
    closeDropdown();
    inputRef.current?.blur();
    router.push({
      pathname: "/(auth)/(tabs)/search",
      params: { q: trimmed, ...(searchKind === "music" ? { kind: "music" } : {}) },
    });
  }, [addToHistory, closeDropdown]);

  const handleFocus = useCallback(() => {
    measure();
    if (query.trim().length > 1 || recents.length > 0) setFocused(true);
  }, [measure, query, recents.length]);

  const handleChangeText = useCallback((value: string) => {
    setQuery(value);
    if (value.trim().length > 1) {
      measure();
      setFocused(true);
    }
  }, [measure]);

  const handleSubmit = useCallback(() => {
    if (!query.trim()) return;
    navigateToFullSearch(query, kind);
  }, [query, kind, navigateToFullSearch]);

  const handleSelectMedia = useCallback((item: V1MediaItem) => {
    setQuery("");
    closeDropdown();
    inputRef.current?.blur();
    router.push(`/media/${item.id}`);
  }, [closeDropdown]);

  const handleSelectTrack = useCallback((tr: V1Track) => {
    closeDropdown();
    inputRef.current?.blur();
    setQueue([{
      id: tr.id ?? "",
      title: tr.title ?? "",
      artist: tr.artist ?? "",
      album: tr.album ?? "",
      duration: tr.duration ?? 0,
    }], 0, searchTrackMusicSource(tr.albumId, tr.album));
  }, [closeDropdown, setQueue]);

  const handlePickRecent = useCallback((q: string) => {
    navigateToFullSearch(q, kind);
  }, [navigateToFullSearch, kind]);

  const overlayTop = layout ? layout.y + layout.height + 4 : 0;

  return (
    <View style={styles.wrapper} ref={wrapperRef} onLayout={measure} collapsable={false}>
      <View style={[styles.inputRow, { backgroundColor: theme.colors.surfaceRaised, borderColor: focused ? theme.colors.accent : theme.colors.border }]}>
        <Ionicons name="search-outline" size={ICON_SIZE.md} color={theme.colors.textMuted} style={{ marginRight: 6 }} />
        <TextInput
          ref={inputRef}
          value={query}
          onChangeText={handleChangeText}
          onFocus={handleFocus}
          onSubmitEditing={handleSubmit}
          placeholder={t("media.search.placeholder")}
          placeholderTextColor={theme.colors.textMuted}
          returnKeyType="search"
          autoCapitalize="none"
          autoCorrect={false}
          style={[styles.input, { color: theme.colors.text }]}
        />
        {query.length > 0 && (
          <Pressable
            onPress={() => { setQuery(""); inputRef.current?.focus(); }}
            accessible
            accessibilityLabel={t("common.clear")}
            accessibilityRole="button"
          >
            <Ionicons name="close-circle" size={ICON_SIZE.md} color={theme.colors.textMuted} />
          </Pressable>
        )}
      </View>

      <Modal
        visible={focused}
        transparent
        animationType="fade"
        presentationStyle="overFullScreen"
        statusBarTranslucent
        onRequestClose={closeDropdown}
      >
        {/* Starts below the header so the field above stays visible and
            tappable; tapping this scrim (results area and beyond) closes
            the panel, like clicking outside the web dropdown. */}
        <Pressable
          style={[styles.backdrop, { top: overlayTop }]}
          onPress={closeDropdown}
          accessibilityElementsHidden
          importantForAccessibility="no-hide-descendants"
        />
        {layout && (
          <SearchDropdown
            style={{ position: "absolute", top: overlayTop, left: layout.x, width: layout.width }}
            query={debouncedQuery}
            visible={focused}
            kind={kind}
            onKindChange={setKind}
            recents={recents}
            onPickRecent={handlePickRecent}
            onRemoveRecent={removeRecent}
            onClearRecents={clearRecents}
            onSelectMedia={handleSelectMedia}
            onSelectTrack={handleSelectTrack}
            onSeeAll={() => navigateToFullSearch(query, kind)}
          />
        )}
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: {
    flex: 1,
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
  backdrop: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
  },
});
