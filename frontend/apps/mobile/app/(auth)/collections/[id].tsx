import { useCallback, useState, useMemo } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Pressable,
  StyleSheet,
  Alert,
  Modal,
  TextInput,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useAuthStore } from "@evey/store";
import { Skeleton } from "../../../src/components/Skeleton";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import {
  useCollection,
  useUpdateCollection,
  useDeleteCollection,
  useRemoveCollectionItem,
  useReorderCollectionItems,
  type CollectionItem,
} from "../../../src/api/collections";
import { artworkUrl } from "../../../src/lib/artwork";
import { moveItem } from "../../../src/utils/collectionReorder";
import { POSTER_GRID } from "../../../src/lib/posterGrid";

const NUM_COLUMNS = POSTER_GRID.columns;
const SPACING = 8;
const ITEM_WIDTH = POSTER_GRID.itemWidth;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;
const ROW_HEIGHT = POSTER_HEIGHT + 30 + SPACING;

function colKeyExtractor(item: CollectionItem) {
  return item.id;
}

// FlatList hands getItemLayout the ROW index when numColumns > 1, so dividing by
// NUM_COLUMNS here divided it twice: consecutive rows reported the same offset,
// and the grid scrolled in jumps instead of flowing. Same defect as the library
// screen, fixed together on 2026-09-04.
const colGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * index,
  index,
});

function EditCollectionModal({
  visible,
  onClose,
  theme,
  initialName,
  initialDescription,
  collectionId,
}: {
  visible: boolean;
  onClose: () => void;
  theme: ReturnType<typeof useTheme>;
  initialName: string;
  initialDescription: string;
  collectionId: string;
}) {
  const { t } = useTranslation();
  const updateMut = useUpdateCollection();
  const [name, setName] = useState(initialName);
  const [description, setDescription] = useState(initialDescription);

  function handleClose() {
    setName(initialName);
    setDescription(initialDescription);
    onClose();
  }

  async function handleSave() {
    if (!name.trim()) return;
    try {
      // Send overview only when it was touched: leaving it out keeps the
      // synopsis a TMDB saga came with, which a rename must not cost it.
      const next = description.trim();
      await updateMut.mutateAsync({
        id: collectionId,
        name: name.trim(),
        ...(next === initialDescription.trim() ? {} : { overview: next }),
      });
      toast.success(t("media.collections.updated"));
      onClose();
    } catch {
      Alert.alert(t("common.error", "Error"), t("common.tryAgain", "Please try again."));
    }
  }

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={handleClose}>
      <KeyboardAvoidingView
        style={styles.modalOverlay}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <View style={[styles.modalContent, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
          <Text style={[styles.modalTitle, { color: theme.colors.text }]}>
            {t("media.collections.editCollection", "Edit Collection")}
          </Text>

          <TextInput
            style={[styles.input, { color: theme.colors.text, borderColor: theme.colors.surfaceRaised, backgroundColor: theme.colors.bg }]}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={t("common.name", "Name")}
            value={name}
            onChangeText={setName}
            autoFocus
            testID="collection-edit-name"
          />

          <TextInput
            style={[styles.input, styles.inputMultiline, { color: theme.colors.text, borderColor: theme.colors.surfaceRaised, backgroundColor: theme.colors.bg }]}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={t("common.description", "Description")}
            value={description}
            onChangeText={setDescription}
            testID="collection-edit-description"
            multiline
            numberOfLines={3}
          />

          <View style={styles.modalButtons}>
            <TouchableOpacity onPress={handleClose} style={[styles.modalBtn, { backgroundColor: theme.colors.surfaceRaised }]}>
              <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel", "Cancel")}</Text>
            </TouchableOpacity>
            <TouchableOpacity
              onPress={handleSave}
              testID="collection-edit-save"
              disabled={!name.trim() || updateMut.isPending}
              style={[styles.modalBtn, { backgroundColor: theme.colors.accent, opacity: !name.trim() || updateMut.isPending ? 0.5 : 1 }]}
            >
              {updateMut.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.onAccent} />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("common.save", "Save")}</Text>
              )}
            </TouchableOpacity>
          </View>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

export default function CollectionDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const { data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useCollection(id ?? "");
  const deleteMut = useDeleteCollection();
  const removeItemMut = useRemoveCollectionItem(id ?? "");
  const reorderMut = useReorderCollectionItems(id ?? "");
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const col = data?.collection;
  const items = useMemo(() => data?.items ?? [], [data?.items]);

  const [editVisible, setEditVisible] = useState(false);

  function confirmDelete() {
    Alert.alert(
      t("media.collections.deleteConfirm", "Delete this collection?"),
      col?.name ?? "",
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: async () => {
            try {
              await deleteMut.mutateAsync(id ?? "");
              toast.success(t("media.collections.deleted"));
              router.back();
            } catch {
              Alert.alert(t("common.error", "Error"), t("common.tryAgain", "Please try again."));
            }
          },
        },
      ],
    );
  }

  const confirmRemoveItem = useCallback(
    (item: CollectionItem) => {
      Alert.alert(
        t("media.collections.removeItem"),
        t("media.collections.removeItemConfirm", { title: item.title }),
        [
          { text: t("common.cancel"), style: "cancel" },
          {
            text: t("media.collections.removeItem"),
            style: "destructive",
            onPress: async () => {
              try {
                await removeItemMut.mutateAsync(item.id);
                toast.success(t("media.collections.itemRemoved"));
                refetch();
              } catch {
                toast.error(t("common.error"));
              }
            },
          },
        ],
      );
    },
    [t, removeItemMut, refetch],
  );

  const renderItem = useCallback(
    ({ item }: { item: CollectionItem }) => {
      const poster = serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 240) : undefined;
      return (
        <View style={{ width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }}>
          <TouchableOpacity
            style={styles.gridItem}
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
            <Text
              style={[styles.title, { color: theme.colors.text }]}
              numberOfLines={2}
            >
              {item.title}
            </Text>
          </TouchableOpacity>
          {isAdmin && items.length > 1 && (
            <View style={[styles.reorderBtns, { backgroundColor: theme.colors.surface }]}>
              <Pressable
                onPress={() =>
                  reorderMut.mutate(
                    { mediaItemIds: moveItem(items.map((i) => i.id), item.id, "up"), name: col?.name ?? "" },
                    { onError: () => toast.error(t("common.error")) },
                  )
                }
                hitSlop={8}
                disabled={reorderMut.isPending || items[0]?.id === item.id}
                style={{ opacity: reorderMut.isPending || items[0]?.id === item.id ? 0.4 : 1 }}
                accessibilityRole="button"
                accessibilityLabel={t("media.collections.moveUp")}
                testID={`collection-up-${item.id}`}
              >
                <Ionicons name="chevron-up-circle" size={20} color={theme.colors.text} />
              </Pressable>
              <Pressable
                onPress={() =>
                  reorderMut.mutate(
                    { mediaItemIds: moveItem(items.map((i) => i.id), item.id, "down"), name: col?.name ?? "" },
                    { onError: () => toast.error(t("common.error")) },
                  )
                }
                hitSlop={8}
                disabled={reorderMut.isPending || items[items.length - 1]?.id === item.id}
                style={{ opacity: reorderMut.isPending || items[items.length - 1]?.id === item.id ? 0.4 : 1 }}
                accessibilityRole="button"
                accessibilityLabel={t("media.collections.moveDown")}
                testID={`collection-down-${item.id}`}
              >
                <Ionicons name="chevron-down-circle" size={20} color={theme.colors.text} />
              </Pressable>
            </View>
          )}
          {isAdmin && (
            <Pressable
              onPress={() => confirmRemoveItem(item)}
              hitSlop={8}
              style={[styles.removeBtn, { backgroundColor: theme.colors.surface }]}
              accessibilityRole="button"
              accessibilityLabel={t("media.collections.removeItem")}
              testID={`collection-remove-${item.id}`}
            >
              <Ionicons name="close-circle" size={20} color={theme.colors.danger} />
            </Pressable>
          )}
        </View>
      );
    },
    [theme, router, serverUrl, authHeaders, t, isAdmin, confirmRemoveItem, items, reorderMut, col?.name],
  );

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: col?.name ?? t("media.collections.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerRight: () =>
            isAdmin ? (
              <View style={{ flexDirection: "row", gap: 12, marginRight: 8 }}>
                <TouchableOpacity onPress={() => setEditVisible(true)} testID="collection-edit-button" accessibilityRole="button" accessibilityLabel={t("common.edit")} hitSlop={8}>
                  <Ionicons name="pencil-outline" size={22} color={theme.colors.text} />
                </TouchableOpacity>
                <TouchableOpacity onPress={confirmDelete} testID="collection-delete-button" accessibilityRole="button" accessibilityLabel={t("common.delete")} hitSlop={8}>
                  <Ionicons name="trash-outline" size={22} color={theme.colors.danger} />
                </TouchableOpacity>
              </View>
            ) : null,
        }}
      />
      {!isError && !isLoading && col?.overview ? (
        <Text style={[styles.overview, { color: theme.colors.textMuted }]} numberOfLines={3}>
          {col.overview}
        </Text>
      ) : null}
      {!isError && !isLoading && items.length > 0 ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, paddingHorizontal: 16, paddingBottom: 4 }}>
          {t("common.itemCount", "{{count}} items", { count: items.length })}
        </Text>
      ) : null}
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
          keyExtractor={colKeyExtractor}
          renderItem={renderItem}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ paddingTop: SPACING }}
          getItemLayout={colGetItemLayout}
          initialNumToRender={9}
          maxToRenderPerBatch={12}
          windowSize={5}
          removeClippedSubviews={true}
          onEndReachedThreshold={0.5}
          onEndReached={() => {
            if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
          }}
          ListEmptyComponent={
            <View style={styles.center}>
              <Text style={{ color: theme.colors.textMuted }}>{t("common.noItems")}</Text>
            </View>
          }
        />
      )}

      {col && (
        <EditCollectionModal
          visible={editVisible}
          onClose={() => setEditVisible(false)}
          theme={theme}
          initialName={col.name}
          initialDescription={col.overview ?? ""}
          collectionId={col.id}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 64 },
  skeletonGrid: { flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING },
  overview: { fontSize: 14, paddingHorizontal: SPACING * 2, paddingTop: SPACING, paddingBottom: SPACING },
  gridItem: { alignItems: "center" },
  title: { fontSize: 12, fontWeight: "500", marginTop: 4, textAlign: "center" },
  removeBtn: { position: "absolute", top: 4, right: 4, borderRadius: 12 },
  reorderBtns: { position: "absolute", top: 4, left: 4, borderRadius: 12, flexDirection: "column", gap: 2 },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.5)",
    justifyContent: "center",
    alignItems: "center",
    padding: 24,
  },
  modalContent: { width: "100%", maxWidth: 400, padding: 20 },
  modalTitle: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  input: {
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 15,
    marginBottom: 12,
  },
  inputMultiline: { minHeight: 72, textAlignVertical: "top" },
  modalButtons: { flexDirection: "row", justifyContent: "flex-end", gap: 10, marginTop: 4 },
  modalBtn: { paddingHorizontal: 18, paddingVertical: 10, borderRadius: 8, alignItems: "center", justifyContent: "center" },
});
