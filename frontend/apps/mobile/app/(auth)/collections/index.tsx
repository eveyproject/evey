import { useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  StyleSheet,
  Dimensions,
  Alert,
  Modal,
  TextInput,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useAuthStore } from "@evey/store";
import { Skeleton } from "../../../src/components/Skeleton";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { SmartRulesEditor, COLLECTION_FIELDS, type SmartRule } from "../../../src/components/SmartRulesEditor";
import { useTheme } from "../../../src/providers/ThemeProvider";
import {
  useCollections,
  useCreateCollection,
  useDeleteCollection,
  type Collection,
} from "../../../src/api/collections";
import { collectionSubtitle, type CollectionSubtitleParts } from "@evey/client";

const NUM_COLUMNS = 2;
const SPACING = 12;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH =
  (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;

// Shared by the four clients: packages/client/src/lib/collectionSubtitle.ts.
export { collectionSubtitle, type CollectionSubtitleParts };

function CollectionCard({
  col,
  theme,
  onPress,
  onLongPress,
}: {
  col: Collection;
  theme: ReturnType<typeof useTheme>;
  onPress: () => void;
  onLongPress?: () => void;
}) {
  const { t } = useTranslation();
  const initial = col.name?.[0]?.toUpperCase() ?? "?";
  const subtitle = collectionSubtitle(col.kind, col.itemCount, t);

  return (
    <TouchableOpacity
      style={[styles.card, { width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }]}
      activeOpacity={0.7}
      onPress={onPress}
      onLongPress={onLongPress}
    >
      {col.posterUrl ? (
        <Image
          source={{ uri: col.posterUrl }}
          style={[
            styles.poster,
            {
              width: ITEM_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.sm,
            },
          ]}
          contentFit="cover"
          transition={200}
        />
      ) : (
        <View
          style={[
            styles.placeholder,
            {
              width: ITEM_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.sm,
              backgroundColor: theme.colors.surfaceRaised,
            },
          ]}
        >
          <Text style={{ color: theme.colors.textMuted, fontSize: 32, fontWeight: "700" }}>
            {initial}
          </Text>
        </View>
      )}
      <View style={{ flexDirection: "row", alignItems: "center", gap: 4, marginTop: 4 }}>
        <Text
          style={[styles.name, { color: theme.colors.text, flex: 1 }]}
          numberOfLines={2}
        >
          {col.name}
        </Text>
        {subtitle.badge && (
          <View style={{ backgroundColor: theme.colors.surfaceRaised, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 9, fontWeight: "700" }}>{subtitle.badge.toUpperCase()}</Text>
          </View>
        )}
        {col.rules && col.rules.length > 0 && (
          <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
            <Text style={{ color: theme.colors.onAccent, fontSize: 9, fontWeight: "700" }}>SMART</Text>
          </View>
        )}
      </View>
      {subtitle.count && (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>
          {subtitle.count}
        </Text>
      )}
    </TouchableOpacity>
  );
}

function CreateCollectionModal({
  visible,
  onClose,
  theme,
}: {
  visible: boolean;
  onClose: () => void;
  theme: ReturnType<typeof useTheme>;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const createMut = useCreateCollection();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  // A smart collection is rule-driven (web parity: New Smart Collection); a
  // manual one holds hand-picked items and takes an optional description.
  const [isSmart, setIsSmart] = useState(false);
  const [rules, setRules] = useState<SmartRule[]>([]);
  const [rulesOpen, setRulesOpen] = useState(false);

  function handleClose() {
    setName("");
    setDescription("");
    setIsSmart(false);
    setRules([]);
    onClose();
  }

  async function handleCreate() {
    if (!name.trim()) return;
    try {
      const res = await createMut.mutateAsync({
        name: name.trim(),
        overview: isSmart ? undefined : description.trim() || undefined,
        rules: isSmart ? rules.filter((r) => r.value.trim()).map((r) => ({ field: r.field, operator: r.operator, value: r.value })) : undefined,
      });
      toast.success(t("media.collections.created"));
      handleClose();
      if (res.id) router.push(isSmart ? `/collections/smart/${res.id}` : `/collections/${res.id}`);
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
            {isSmart ? t("media.collections.newSmartCollection", "New Smart Collection") : t("media.collections.newCollection", "New Collection")}
          </Text>

          {/* Manual vs smart segmented toggle */}
          <View style={{ flexDirection: "row", gap: 8, marginBottom: 12 }}>
            {[{ smart: false, label: t("media.playlist.manual", "Manual") }, { smart: true, label: t("media.playlist.smart", "Smart") }].map((opt) => (
              <TouchableOpacity
                key={String(opt.smart)}
                onPress={() => setIsSmart(opt.smart)}
                style={{ flex: 1, alignItems: "center", paddingVertical: 8, borderRadius: theme.radii.sm, backgroundColor: isSmart === opt.smart ? theme.colors.accent : theme.colors.surfaceRaised }}
                accessibilityRole="radio"
                accessibilityState={{ selected: isSmart === opt.smart }}
                accessibilityLabel={opt.label}
              >
                <Text style={{ color: isSmart === opt.smart ? theme.colors.onAccent : theme.colors.text, fontWeight: "600", fontSize: 13 }}>{opt.label}</Text>
              </TouchableOpacity>
            ))}
          </View>

          <TextInput
            style={[styles.input, { color: theme.colors.text, borderColor: theme.colors.surfaceRaised, backgroundColor: theme.colors.bg }]}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={t("common.name", "Name")}
            value={name}
            onChangeText={setName}
            autoFocus
          />

          {isSmart ? (
            <TouchableOpacity
              onPress={() => setRulesOpen(true)}
              style={[styles.input, { flexDirection: "row", alignItems: "center", justifyContent: "space-between", borderColor: theme.colors.surfaceRaised, backgroundColor: theme.colors.bg }]}
              accessibilityRole="button"
              accessibilityLabel={t("media.playlist.smartRules", "Smart rules")}
            >
              <Text style={{ color: theme.colors.text }}>
                {t("media.playlist.smartRules", "Smart rules")} ({rules.filter((r) => r.value.trim()).length})
              </Text>
              <Ionicons name="options-outline" size={18} color={theme.colors.accent} />
            </TouchableOpacity>
          ) : (
            <TextInput
              style={[styles.input, styles.inputMultiline, { color: theme.colors.text, borderColor: theme.colors.surfaceRaised, backgroundColor: theme.colors.bg }]}
              placeholderTextColor={theme.colors.textMuted}
              placeholder={t("common.description", "Description")}
              value={description}
              onChangeText={setDescription}
              multiline
              numberOfLines={3}
            />
          )}

          <View style={styles.modalButtons}>
            <TouchableOpacity onPress={handleClose} style={[styles.modalBtn, { backgroundColor: theme.colors.surfaceRaised }]}>
              <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel", "Cancel")}</Text>
            </TouchableOpacity>
            <TouchableOpacity
              onPress={handleCreate}
              disabled={!name.trim() || createMut.isPending}
              style={[styles.modalBtn, { backgroundColor: theme.colors.accent, opacity: !name.trim() || createMut.isPending ? 0.5 : 1 }]}
            >
              {createMut.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.onAccent} />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("common.create", "Create")}</Text>
              )}
            </TouchableOpacity>
          </View>
        </View>
      </KeyboardAvoidingView>
      {isSmart && (
        <SmartRulesEditor
          visible={rulesOpen}
          onClose={() => setRulesOpen(false)}
          rules={rules}
          onSave={setRules}
          availableFields={COLLECTION_FIELDS}
        />
      )}
    </Modal>
  );
}

export default function CollectionsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data, isLoading, isError, refetch } = useCollections();
  const deleteMut = useDeleteCollection();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const [createVisible, setCreateVisible] = useState(false);

  const collections = data?.collections ?? [];

  function confirmDelete(col: Collection) {
    Alert.alert(
      t("media.collections.deleteConfirm", "Delete this collection?"),
      col.name,
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: () =>
            deleteMut.mutate(col.id, {
              onSuccess: () => toast.success(t("media.collections.deleted")),
              onError: () => toast.error(t("media.collections.deleteFailed")),
            }),
        },
      ],
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("media.collections.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerRight: () =>
            isAdmin ? (
              <TouchableOpacity onPress={() => setCreateVisible(true)} style={{ marginRight: 8 }} hitSlop={8} accessibilityRole="button" accessibilityLabel={t("media.collections.newCollection")}>
                <Ionicons name="add-circle-outline" size={26} color={theme.colors.accent} />
              </TouchableOpacity>
            ) : null,
        }}
      />
      {isLoading ? (
        <View style={styles.skeletonGrid}>
          {Array.from({ length: 6 }).map((_, i) => (
            <View key={i} style={{ marginLeft: SPACING, marginBottom: SPACING }}>
              <Skeleton width={ITEM_WIDTH} height={POSTER_HEIGHT} borderRadius={theme.radii.sm} />
              <Skeleton width={ITEM_WIDTH * 0.7} height={12} borderRadius={4} style={{ marginTop: 6 }} />
            </View>
          ))}
        </View>
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
      ) : (
        <FlatList
          data={collections}
          keyExtractor={(item) => item.id}
          numColumns={NUM_COLUMNS}
          contentContainerStyle={{ paddingTop: SPACING }}
          refreshing={false}
          onRefresh={refetch}
          renderItem={({ item }) => (
            <CollectionCard
              col={item}
              theme={theme}
              onPress={() => {
                if (item.rules && item.rules.length > 0) {
                  router.push(`/collections/smart/${item.id}`);
                } else {
                  router.push(`/collections/${item.id}`);
                }
              }}
              onLongPress={isAdmin ? () => confirmDelete(item) : undefined}
            />
          )}
          ListEmptyComponent={
            <View style={styles.center}>
              <Ionicons name="folder-outline" size={48} color={theme.colors.textMuted} style={{ marginBottom: 10 }} />
              <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", textAlign: "center" }}>{t("media.collections.noCollections")}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center", marginTop: 6, paddingHorizontal: 24 }}>{t("media.collections.noCollectionsDesc")}</Text>
            </View>
          }
        />
      )}

      <CreateCollectionModal
        visible={createVisible}
        onClose={() => setCreateVisible(false)}
        theme={theme}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 64 },
  skeletonGrid: { flexDirection: "row", flexWrap: "wrap", paddingTop: SPACING },
  card: { alignItems: "center" },
  poster: { width: "100%" },
  placeholder: { justifyContent: "center", alignItems: "center" },
  name: { fontSize: 13, fontWeight: "600", marginTop: 4, textAlign: "center" },
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
