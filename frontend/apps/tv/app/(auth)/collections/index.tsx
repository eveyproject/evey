import { View, Text, FlatList, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useCollections, type Collection } from "../../../src/api/collections";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { ScreenState } from "../../../src/components/ScreenState";

const NUM_COLUMNS = 5;

export default function CollectionsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const { data, isLoading, isError, refetch } = useCollections();

  const collections = data?.collections ?? [];

  // "Empty" and "the server did not answer" are different sentences, and this
  // screen used to say the first for both.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("common.collections")}</Text>
      <FlatList
        data={collections}
        keyExtractor={(item) => item.id}
        numColumns={NUM_COLUMNS}
        contentContainerStyle={{ padding: 24 }}
        ItemSeparatorComponent={() => <View style={{ height: 16 }} />}
        renderItem={({ item, index }) => (
          <CollectionCard
            col={item}
            theme={theme}
            isFirst={index === 0}
            onPress={() => router.push(`/collections/${item.id}`)}
          />
        )}
        ListEmptyComponent={
          <View style={styles.center}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 22, marginBottom: 16 }}>{t("common.noCollections")}</Text>
            {/* Focusable target for the empty state - without it the remote has
                nowhere to go and gets stuck on this screen (no other control
                is rendered when the list is empty). */}
            <FocusableCard
              onPress={() => router.push("/(auth)/(tabs)/library")}
              hasTVPreferredFocus
              style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
              accessibilityLabel={t("media.library.libraries")}
            >
              <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
            </FocusableCard>
          </View>
        }
      />
    </View>
  );
}

function CollectionCard({
  col,
  theme,
  isFirst,
  onPress,
}: {
  col: Collection;
  theme: ReturnType<typeof useTheme>;
  isFirst?: boolean;
  onPress: () => void;
}) {
  const initial = col.name[0]?.toUpperCase() ?? "?";

  return (
    <FocusableCard
      onPress={onPress}
      hasTVPreferredFocus={isFirst}
      style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      accessibilityLabel={col.name}
    >
      <View style={[styles.placeholder, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 36, fontWeight: "700" }}>{initial}</Text>
      </View>
      <Text style={[styles.name, { color: theme.colors.text }]} numberOfLines={2}>
        {col.name}
      </Text>
    </FocusableCard>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  header: { fontSize: 36, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32, paddingBottom: 16 },
  card: { flex: 1, margin: 8, padding: 12, alignItems: "center" },
  placeholder: { width: 160, height: 240, justifyContent: "center", alignItems: "center" },
  name: { fontSize: 18, fontWeight: "600", marginTop: 8, textAlign: "center" },
  emptyAction: { paddingHorizontal: 24, paddingVertical: 14 },
});
