import { View, Text, FlatList, StyleSheet } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { useMediaItemCollections } from "../api/collections";
import { FocusableCard } from "./FocusableCard";

/**
 * The sagas a title belongs to, on the title's own page. Mirrors the web's
 * MediaDetailSagas and mobile's SagaSection: renders nothing while loading or
 * when the title belongs to nothing, which is the common case.
 */
export function SagaSection({ mediaItemId }: { mediaItemId: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { data } = useMediaItemCollections(mediaItemId);
  const collections = data?.collections ?? [];
  if (collections.length === 0) return null;

  return (
    <View style={{ marginTop: 32 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
        {t("media.collections.partOf")}
      </Text>
      <FlatList
        horizontal
        data={collections}
        keyExtractor={(c) => c.id}
        showsHorizontalScrollIndicator={false}
        ItemSeparatorComponent={() => <View style={{ width: 16 }} />}
        renderItem={({ item: c }) => (
          <FocusableCard onPress={() => router.push(`/collections/${c.id}`)} style={styles.card} accessibilityLabel={c.name}>
            <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }} numberOfLines={1}>
              {c.name}
            </Text>
            {(c.itemCount ?? 0) > 0 ? (
              <Text style={{ color: theme.colors.textMuted, fontSize: 14, marginTop: 4 }}>
                {t("media.person.titleCount", { count: c.itemCount })}
              </Text>
            ) : null}
          </FocusableCard>
        )}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  sectionTitle: { fontSize: 24, fontWeight: "600", marginBottom: 12 },
  card: { paddingVertical: 11, paddingHorizontal: 24 },
});
