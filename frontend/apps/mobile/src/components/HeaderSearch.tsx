import { useState, useRef, useCallback } from "react";
import { View, TextInput, Text, Pressable, Modal, StyleSheet } from "react-native";
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
// panel is therefore rendered in a transparent `Modal`, its own native surface
// (not a toolbar descendant), positioned by measuring this component's wrapper
// in window coordinates so it lands directly under the field.
//
// CRITICAL: the search TextInput lives INSIDE that Modal, not in the header. A
// RN Modal is a separate native window; presenting it while a TextInput in the
// PARENT window holds focus makes Android drop that window's focus and dismiss
// the soft keyboard - so a header-hosted input lost its keyboard the instant
// the panel opened (reported: "je n'ai plus le clavier", the panel opening on
// focus once history existed). With the input in the Modal, the keyboard
// belongs to the Modal's own window and stays up. The header shows a Pressable
// that only mirrors the field and opens the panel.
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
  const [open, setOpen] = useState(false);
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

  // Tapping the header field opens the panel; the Modal's input then grabs the
  // keyboard (see the file header for why the input must live in the Modal).
  const openPanel = useCallback(() => {
    measure();
    setOpen(true);
  }, [measure]);

  const closeDropdown = useCallback(() => {
    setOpen(false);
    setQuery("");
    inputRef.current?.blur();
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
    closeDropdown();
    router.push({
      pathname: "/(auth)/(tabs)/search",
      params: { q: trimmed, ...(searchKind === "music" ? { kind: "music" } : {}) },
    });
  }, [addToHistory, closeDropdown]);

  const handleChangeText = useCallback((value: string) => {
    setQuery(value);
  }, []);

  const handleSubmit = useCallback(() => {
    if (!query.trim()) return;
    navigateToFullSearch(query, kind);
  }, [query, kind, navigateToFullSearch]);

  const handleSelectMedia = useCallback((item: V1MediaItem) => {
    closeDropdown();
    router.push(`/media/${item.id}`);
  }, [closeDropdown]);

  const handleSelectTrack = useCallback((tr: V1Track) => {
    closeDropdown();
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

  const inputRow = (interactive: boolean) => (
    <View style={[styles.inputRow, { backgroundColor: theme.colors.surfaceRaised, borderColor: interactive ? theme.colors.accent : theme.colors.border }]}>
      <Ionicons name="search-outline" size={ICON_SIZE.md} color={theme.colors.textMuted} style={{ marginRight: 6 }} />
      {interactive ? (
        <>
          <TextInput
            ref={inputRef}
            value={query}
            onChangeText={handleChangeText}
            onSubmitEditing={handleSubmit}
            placeholder={t("media.search.placeholder")}
            placeholderTextColor={theme.colors.textMuted}
            returnKeyType="search"
            autoCapitalize="none"
            autoCorrect={false}
            autoFocus
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
        </>
      ) : (
        <Text style={[styles.input, { color: theme.colors.textMuted }]} numberOfLines={1}>
          {t("media.search.placeholder")}
        </Text>
      )}
    </View>
  );

  return (
    <View style={styles.wrapper} ref={wrapperRef} onLayout={measure} collapsable={false}>
      {/* Header field: a trigger only. Tapping it opens the panel whose Modal
          hosts the real input - keeping the focused input out of the toolbar's
          window is what preserves the keyboard. */}
      <Pressable onPress={openPanel} accessibilityRole="search">
        {inputRow(false)}
      </Pressable>

      <Modal
        visible={open}
        transparent
        animationType="fade"
        presentationStyle="overFullScreen"
        statusBarTranslucent
        onRequestClose={closeDropdown}
        onShow={() => inputRef.current?.focus()}
      >
        {/* Full backdrop behind the panel: tapping outside closes it, like
            clicking outside the web dropdown. */}
        <Pressable
          style={styles.backdrop}
          onPress={closeDropdown}
          accessibilityElementsHidden
          importantForAccessibility="no-hide-descendants"
        />
        {open && layout && (
          <View style={{ position: "absolute", top: layout.y, left: layout.x, width: layout.width }}>
            {inputRow(true)}
            <SearchDropdown
              style={{ marginTop: 4 }}
              query={debouncedQuery}
              visible={open}
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
          </View>
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
    ...StyleSheet.absoluteFillObject,
  },
});
