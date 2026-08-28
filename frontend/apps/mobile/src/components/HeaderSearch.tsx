import { useState, useRef, useCallback, useEffect } from "react";
import { View, TextInput, Text, Pressable, Modal, Keyboard, StyleSheet } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
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
// clip a toolbar's children to the bar's bounds - so the results panel renders
// in a transparent `Modal`, its own native surface. The header shows a
// Pressable that only mirrors the field; the real TextInput lives INSIDE the
// Modal (a header-hosted input loses its keyboard the instant the Modal
// opens, because Android moves window focus to the Modal's window).
//
// The Modal content is a plain full-screen layout padded by the safe-area
// insets and the live keyboard height. Do NOT try to mirror the header
// field's on-screen position by measuring it (measureInWindow) and replaying
// the coordinates inside the Modal: the app window's and the Modal window's
// coordinate spaces disagree per OEM/edge-to-edge mode, which pushed the
// panel into the status-bar/notch and gesture areas on device. Percentage
// heights also never resolved under that absolutely-positioned wrapper, so
// the panel height was unbounded.
//
// CRITICAL (keyboard): `autoFocus` on a TextInput inside a Modal is a known
// Android bug - the IME drops focus requests issued while the dialog window
// is still settling, so the keyboard never shows (facebook/react-native
// #26297). The input must be mounted for the whole life of the Modal and
// focused from `onShow`, deferred by a tick.
//
// Shares its recent-searches storage (SecureStore key below) with the full
// search screen ((tabs)/search.tsx) so a query saved from either surface
// shows up in both.
const HISTORY_KEY = "evey-search-history";

// Deferred focus from onShow; 100ms lets the dialog window finish gaining
// window focus before the IME request, across OEMs (ColorOS included).
const FOCUS_DELAY_MS = 100;

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

// The Modal window does not resize with the soft keyboard (statusBarTranslucent
// disables adjustResize on Android), so the overlay pads its bottom by the
// measured keyboard height instead.
function useKeyboardHeight(): number {
  const [height, setHeight] = useState(0);
  useEffect(() => {
    const show = Keyboard.addListener("keyboardDidShow", (e) => setHeight(e.endCoordinates.height));
    const hide = Keyboard.addListener("keyboardDidHide", () => setHeight(0));
    return () => {
      show.remove();
      hide.remove();
    };
  }, []);
  return height;
}

export function HeaderSearch() {
  const theme = useTheme();
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const keyboardHeight = useKeyboardHeight();
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<"media" | "music">("media");
  const [recents, setRecents] = useState<string[]>(() => loadSearchHistory());
  const inputRef = useRef<TextInput>(null);
  const debouncedQuery = useDebounce(query.trim(), 250);
  const setQueue = useMusicQueueStore((s) => s.setQueue);

  const openPanel = useCallback(() => {
    setOpen(true);
  }, []);

  const focusInput = useCallback(() => {
    setTimeout(() => inputRef.current?.focus(), FOCUS_DELAY_MS);
  }, []);

  const closeDropdown = useCallback(() => {
    setOpen(false);
    setQuery("");
    Keyboard.dismiss();
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
    <View style={styles.wrapper}>
      {/* Header field: a trigger only. Tapping it opens the overlay whose Modal
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
        navigationBarTranslucent
        onRequestClose={closeDropdown}
        onShow={focusInput}
      >
        {/* Full backdrop behind the panel: tapping outside closes it, like
            clicking outside the web dropdown. */}
        <Pressable
          style={styles.backdrop}
          onPress={closeDropdown}
          accessibilityElementsHidden
          importantForAccessibility="no-hide-descendants"
        />
        {open && (
          <View
            style={[styles.overlay, {
              paddingTop: insets.top + 8,
              paddingLeft: insets.left + 12,
              paddingRight: insets.right + 12,
              paddingBottom: Math.max(insets.bottom, keyboardHeight) + 8,
            }]}
          >
            {inputRow(true)}
            <SearchDropdown
              style={styles.panelInOverlay}
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
  // Full-screen column; empty space must stay tappable-through to the
  // backdrop underneath, hence box-none.
  overlay: {
    ...StyleSheet.absoluteFillObject,
    pointerEvents: "box-none",
  },
  // The panel caps itself at 70% but that percentage only resolves against a
  // definite parent height; flexShrink additionally clamps it to the space
  // left above the keyboard.
  panelInOverlay: {
    marginTop: 4,
    flexShrink: 1,
  },
});
