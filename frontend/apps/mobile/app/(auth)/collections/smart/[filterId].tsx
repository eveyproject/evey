import { useCallback, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Pressable,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useAuthStore } from "@evey/store";
import { Skeleton } from "../../../../src/components/Skeleton";
import { SmartRulesEditor, COLLECTION_FIELDS, type SmartRule } from "../../../../src/components/SmartRulesEditor";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../../src/hooks/useAuthenticatedImage";
import { useCollection, useUpdateCollection, type CollectionItem } from "../../../../src/api/collections";
import { artworkUrl } from "../../../../src/lib/artwork";
import { POSTER_GRID } from "../../../../src/lib/posterGrid";

const NUM_COLUMNS = POSTER_GRID.columns;
const SPACING = 8;
const ITEM_WIDTH = POSTER_GRID.itemWidth;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;

const OP_LABELS: Record<string, string> = {
  eq: "=",
  gte: ">=",
  lte: "<=",
  gt: ">",
  lt: "<",
};

export default function SmartFilterScreen() {
  const { filterId } = useLocalSearchParams<{ filterId: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  // The header counts what the SERVER holds while the grid only ever rendered
  // the first page of 50 - a 312-item smart collection stopped dead at 50 with
  // no "load more" and nothing saying so, which reads as "that is everything".
  // useCollection is already paged; this screen simply never asked for page 2.
  const { data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useCollection(filterId ?? "");
  const col = data?.collection;
  const items = data?.items ?? [];
  const rules = col?.rules ?? [];
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const updateMut = useUpdateCollection();
  const [editOpen, setEditOpen] = useState(false);

  const renderItem = useCallback(
    ({ item }: { item: CollectionItem }) => {
      const poster = serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 240) : undefined;
      return (
        <TouchableOpacity
          style={[styles.gridItem, { width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }]}
          activeOpacity={0.7}
          onPress={() => router.push(`/media/${item.id}`)}
        >
          {poster ? (
            <Image
              source={{ uri: poster, headers: authHeaders }}
              style={{ width: ITEM_WIDTH, height: POSTER_HEIGHT, borderRadius: theme.radii.sm }}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View
              style={{
                width: ITEM_WIDTH,
                height: POSTER_HEIGHT,
                borderRadius: theme.radii.sm,
                backgroundColor: theme.colors.surfaceRaised,
                justifyContent: "center",
                alignItems: "center",
              }}
            >
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("common.noPoster")}</Text>
            </View>
          )}
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
            {item.title}
          </Text>
          {item.year ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{item.year}</Text>
          ) : null}
        </TouchableOpacity>
      );
    },
    [theme, router, serverUrl, authHeaders, t],
  );

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: col?.name ?? t("media.collections.smart", "Smart Filter"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerRight: () =>
            isAdmin && col ? (
              <TouchableOpacity onPress={() => setEditOpen(true)} hitSlop={8} style={{ marginRight: 8 }} accessibilityRole="button" accessibilityLabel={t("media.playlist.editRules", "Edit rules")}>
                <Ionicons name="options-outline" size={24} color={theme.colors.accent} />
              </TouchableOpacity>
            ) : null,
        }}
      />

      {editOpen && col && (
        <SmartRulesEditor
          visible={editOpen}
          onClose={() => setEditOpen(false)}
          rules={rules.map((r) => ({ field: r.field as SmartRule["field"], operator: (r.operator || "eq") as SmartRule["operator"], value: r.value }))}
          onSave={(newRules) => {
            updateMut.mutate(
              { id: filterId ?? "", name: col.name ?? "", rules: newRules.map((r) => ({ field: r.field, operator: r.operator, value: r.value })) },
              {
                onSuccess: () => {
                  toast.success(t("media.collections.updated"));
                  void refetch();
                },
                onError: () => toast.error(t("media.collections.updateFailed")),
              },
            );
          }}
          availableFields={COLLECTION_FIELDS}
        />
      )}

      {/* Rules badges */}
      {rules.length > 0 && (
        <View style={styles.rulesRow}>
          {rules.map((r, i) => (
            <View
              key={i}
              style={[styles.badge, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}
            >
              <Text style={{ color: theme.colors.text, fontSize: 12 }}>
                {r.field} {OP_LABELS[r.operator] ?? r.operator} {r.value}
              </Text>
            </View>
          ))}
        </View>
      )}

      {isError ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, marginBottom: 12 }}>{t("common.loadError")}</Text>
          <Pressable onPress={() => refetch()}>
            <Text style={{ color: theme.colors.accent }}>{t("common.retry")}</Text>
          </Pressable>
        </View>
      ) : isLoading ? (
        <View style={styles.skeletonGrid}>
          {Array.from({ length: 9 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING, alignItems: "center", width: ITEM_WIDTH }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => item.id}
          renderItem={renderItem}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ paddingTop: SPACING }}
          onEndReachedThreshold={0.5}
          onEndReached={() => {
            if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
          }}
          ListHeaderComponent={
            data ? (
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginLeft: SPACING, marginBottom: SPACING }}>
                {t("common.item", { count: data.total ?? items.length })}
              </Text>
            ) : null
          }
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>
                {rules.length === 0
                  ? t("media.collections.noRules", "No filter rules defined")
                  : t("media.collections.noRuleMatch", "No items match these rules")}
              </Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 64 },
  skeletonGrid: { flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING },
  gridItem: { alignItems: "center" },
  title: { fontSize: 12, fontWeight: "500", marginTop: 4, textAlign: "center" },
  rulesRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 6,
    paddingHorizontal: SPACING,
    paddingVertical: 8,
  },
  badge: {
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
});
