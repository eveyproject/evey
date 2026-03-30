import { useState } from "react";
import {
  View,
  Text,
  TextInput,
  FlatList,
  TouchableOpacity,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Skeleton } from "../../../src/components/Skeleton";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceSearchMedia } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDebounce } from "../../../src/hooks/useDebounce";

function getPoster(item: V1MediaItem): string | undefined {
  return item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url;
}

function formatType(type?: string): string {
  if (!type) return "";
  return type.replace("MEDIA_ITEM_TYPE_", "").toLowerCase();
}

export default function SearchScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounce(query, 300);

  const { data, isLoading } = useLibraryServiceSearchMedia(
    { query: debouncedQuery, limit: 30 },
    { query: { enabled: debouncedQuery.length > 0 } },
  );

  const items = data?.items ?? [];

  const renderItem = ({ item }: { item: V1MediaItem }) => {
    const poster = getPoster(item);
    return (
      <TouchableOpacity
        style={[
          styles.row,
          {
            backgroundColor: theme.colors.surface,
            borderRadius: theme.radii.md,
          },
        ]}
        activeOpacity={0.7}
        onPress={() => router.push(`/media/${item.id}`)}
      >
        {poster ? (
          <Image
            source={{ uri: poster }}
            style={[styles.thumb, { borderRadius: theme.radii.sm }]}
            contentFit="cover"
            transition={200}
          />
        ) : (
          <View
            style={[
              styles.thumb,
              {
                borderRadius: theme.radii.sm,
                backgroundColor: theme.colors.surfaceRaised,
                justifyContent: "center",
                alignItems: "center",
              },
            ]}
          >
            <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
              ?
            </Text>
          </View>
        )}
        <View style={styles.info}>
          <Text
            style={[styles.title, { color: theme.colors.text }]}
            numberOfLines={1}
          >
            {item.title}
          </Text>
          <Text style={[styles.meta, { color: theme.colors.textMuted }]}>
            {[item.year, formatType(item.type)].filter(Boolean).join(" · ")}
          </Text>
        </View>
      </TouchableOpacity>
    );
  };

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <TextInput
        style={[
          styles.input,
          {
            backgroundColor: theme.colors.surface,
            color: theme.colors.text,
            borderRadius: theme.radii.md,
            marginTop: theme.spacing.xl,
          },
        ]}
        placeholder={t("media.search.placeholder")}
        placeholderTextColor={theme.colors.textMuted}
        value={query}
        onChangeText={setQuery}
        autoCapitalize="none"
        autoCorrect={false}
        returnKeyType="search"
      />

      {isLoading && debouncedQuery.length > 0 ? (
        <View style={{ padding: theme.spacing.md }}>
          {Array.from({ length: 6 }).map((_, i) => (
            <View key={i} style={{ flexDirection: "row", alignItems: "center", padding: 10, marginBottom: theme.spacing.sm }}>
              <Skeleton width={48} height={72} borderRadius={theme.radii.sm} />
              <View style={{ flex: 1, marginLeft: 12 }}>
                <Skeleton width="70%" height={15} borderRadius={4} />
                <Skeleton width="40%" height={12} borderRadius={4} style={{ marginTop: 6 }} />
              </View>
            </View>
          ))}
        </View>
      ) : debouncedQuery.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 15 }}>
            {t("media.search.searchLibrary")}
          </Text>
        </View>
      ) : items.length === 0 ? (
        <View style={styles.center}>
          <Text style={{ color: theme.colors.textMuted }}>{t("media.search.noResults")}</Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id ?? ""}
          renderItem={renderItem}
          contentContainerStyle={{ padding: theme.spacing.md }}
          ItemSeparatorComponent={() => (
            <View style={{ height: theme.spacing.sm }} />
          )}
          keyboardShouldPersistTaps="handled"
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  input: {
    marginHorizontal: 16,
    marginBottom: 8,
    paddingHorizontal: 14,
    paddingVertical: 12,
    fontSize: 16,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    padding: 10,
  },
  thumb: { width: 48, height: 72, marginRight: 12 },
  info: { flex: 1 },
  title: { fontSize: 15, fontWeight: "600" },
  meta: { fontSize: 12, marginTop: 2 },
});
