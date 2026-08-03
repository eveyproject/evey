import { View, Text, Pressable } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { useMediaItemCollections } from "../../api/collections";

/**
 * The sagas a title belongs to, on the title's own page. Mirrors the web's
 * MediaDetailSagas: renders nothing while loading or when the title belongs
 * to nothing, which is the common case.
 */
export function SagaSection({ mediaItemId }: { mediaItemId: string }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { data } = useMediaItemCollections(mediaItemId);
  const collections = data?.collections ?? [];
  if (collections.length === 0) return null;

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
        {t("media.collections.partOf")}
      </Text>
      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.sm }}>
        {collections.map((c) => (
          <Pressable
            key={c.id}
            onPress={() => router.push(`/collections/${c.id}`)}
            style={{
              flexDirection: "row",
              alignItems: "center",
              gap: 6,
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
            }}
            accessible={true}
            accessibilityLabel={c.name}
            accessibilityRole="button"
          >
            <Ionicons name="folder-outline" size={16} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{c.name}</Text>
            {(c.itemCount ?? 0) > 0 ? (
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                {t("media.person.titleCount", { count: c.itemCount })}
              </Text>
            ) : null}
          </Pressable>
        ))}
      </View>
    </View>
  );
}
