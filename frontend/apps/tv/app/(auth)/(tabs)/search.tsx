import { useState } from "react";
import { View, Text, TextInput, FlatList, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { FocusableCard } from "../../../src/components/FocusableCard";

function formatType(type?: string): string {
  if (!type) return "";
  return type.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();
}

export default function SearchScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [inputFocused, setInputFocused] = useState(false);
  const debouncedQuery = useDebounce(query, 300);

  const { data, isLoading } = useLibraryServiceSearchMedia(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 0 } },
  );

  const items = data?.items ?? [];

  const renderItem = ({ item, index }: { item: V1MediaItem; index: number }) => (
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
          {[item.year, formatType(item.type)].filter(Boolean).join(" . ")}
        </Text>
      </View>
    </FocusableCard>
  );

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

      {isLoading && debouncedQuery.length > 0 ? (
        <View style={styles.center}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : debouncedQuery.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>
            {t("media.search.searchLibrary")}
          </Text>
        </View>
      ) : items.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 22 }}>{t("media.search.noResults")}</Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id ?? ""}
          renderItem={renderItem}
          contentContainerStyle={{ padding: 24 }}
          ItemSeparatorComponent={() => <View style={{ height: 12 }} />}
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
  info: { flex: 1 },
  title: { fontSize: 22, fontWeight: "600" },
  meta: { fontSize: 16, marginTop: 4 },
});
