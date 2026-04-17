import { useState, useRef, useCallback } from "react";
import { View, Text, TextInput, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useDebounce } from "../hooks/useDebounce";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { useServerUrl } from "../hooks/useServerUrl";

export function HeaderSearch() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [focused, setFocused] = useState(false);
  const inputRef = useRef<TextInput>(null);
  const debouncedQuery = useDebounce(query, 250);
  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();

  const { data } = useLibraryServiceSearchMedia(
    { query: debouncedQuery, limit: 6 },
    { query: { enabled: debouncedQuery.trim().length > 1 } }
  );

  const results = data?.items ?? [];
  const showDropdown = focused && debouncedQuery.trim().length > 1;

  const handleSelect = useCallback((item: V1MediaItem) => {
    setQuery("");
    setFocused(false);
    inputRef.current?.blur();
    router.push(`/media/${item.id}`);
  }, []);

  const handleSubmit = useCallback(() => {
    if (!query.trim()) return;
    setFocused(false);
    inputRef.current?.blur();
    router.push({ pathname: "/(auth)/(tabs)/search", params: { q: query.trim() } });
  }, [query]);

  return (
    <View style={styles.wrapper}>
      <View style={[styles.inputRow, { backgroundColor: theme.colors.surfaceRaised, borderColor: focused ? theme.colors.accent : theme.colors.border }]}>
        <Ionicons name="search-outline" size={15} color={theme.colors.textMuted} style={{ marginRight: 6 }} />
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
            <Ionicons name="close-circle" size={16} color={theme.colors.textMuted} />
          </Pressable>
        )}
      </View>

      {showDropdown && (
        <View style={[styles.dropdown, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, shadowColor: theme.colors.shadow }]}>
          {results.length === 0 ? (
            <View style={styles.emptyRow}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("media.search.noResults", "No results")}</Text>
            </View>
          ) : (
            <>
              {results.map((item) => {
                const posterUrl = `${serverUrl}/api/v1/media/${item.id}/artwork/primary?width=60`;
                return (
                  <Pressable key={item.id} onPress={() => handleSelect(item)} style={styles.resultRow} accessible accessibilityLabel={item.title ?? ""} accessibilityRole="button">
                    <Image
                      source={{ uri: posterUrl, headers: authHeaders }}
                      style={styles.poster}
                      contentFit="cover"
                    />
                    <View style={{ flex: 1 }}>
                      <Text style={[styles.resultTitle, { color: theme.colors.text }]} numberOfLines={1}>{item.title}</Text>
                      <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                        {item.type?.replace("MEDIA_ITEM_TYPE_", "").toLowerCase()}
                        {item.year ? ` · ${item.year}` : ""}
                      </Text>
                    </View>
                    <Ionicons name="chevron-forward" size={14} color={theme.colors.textMuted} />
                  </Pressable>
                );
              })}
              <Pressable onPress={handleSubmit} style={[styles.seeAllRow, { borderTopColor: theme.colors.border }]}>
                <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "500" }}>
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
    fontSize: 14,
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
    fontSize: 14,
    fontWeight: "500",
  },
  seeAllRow: {
    borderTopWidth: 1,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
});
