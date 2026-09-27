import { memo } from "react";
import { View, Text, Pressable } from "react-native";
import { Image } from "expo-image";
import { router } from "expo-router";
import type { V1MediaPerson } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

export const CastCard = memo(function CastCard({ person }: { person: V1MediaPerson }) {
  const theme = useTheme();
  const personType = person.type?.replace("PERSON_TYPE_", "") ?? "";
  const roleLabel = personType.charAt(0) + personType.slice(1).toLowerCase();

  return (
    <Pressable
      onPress={() => person.id && router.push(`/(auth)/person/${person.id}`)}
      style={{ width: 80, marginRight: theme.spacing.sm, alignItems: "center" }}
      accessible={true}
      accessibilityLabel={person.name ?? "Person"}
      accessibilityRole="button"
    >
      {person.thumbUrl ? (
        <Image
          source={{ uri: person.thumbUrl }}
          style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface }}
          contentFit="cover"
          transition={200}
        />
      ) : (
        <View style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>
            {person.name ? person.name[0].toUpperCase() : "?"}
          </Text>
        </View>
      )}
      <Text style={{ color: theme.colors.text, fontSize: 11, marginTop: 4, textAlign: "center" }} numberOfLines={1}>
        {person.name}
      </Text>
      {person.role ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {person.role}
        </Text>
      ) : personType && personType !== "ACTOR" ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {roleLabel}
        </Text>
      ) : null}
    </Pressable>
  );
});

export function personKeyExtractor(p: V1MediaPerson) {
  return p.id ?? p.name ?? "";
}

export function renderCastItem({ item }: { item: V1MediaPerson }) {
  return <CastCard person={item} />;
}
