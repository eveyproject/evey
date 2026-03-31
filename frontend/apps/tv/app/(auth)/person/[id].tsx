import { useState } from "react";
import { View, Text, ScrollView, ActivityIndicator, StyleSheet, Dimensions } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch, useLibraryServiceListMediaItems } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";

const { width: SCREEN_WIDTH } = Dimensions.get("window");
const POSTER_COLS = 5;
const POSTER_GAP = 16;
const PAD_H = 48;
const POSTER_WIDTH = (SCREEN_WIDTH - PAD_H * 2 - POSTER_GAP * (POSTER_COLS - 1)) / POSTER_COLS;
const POSTER_HEIGHT = POSTER_WIDTH * 1.5;

interface PersonInfo {
  id?: string;
  name?: string;
  thumbUrl?: string;
  biography?: string;
  birthday?: string;
  deathday?: string;
  placeOfBirth?: string;
}

function useGetPerson(id: string) {
  return useQuery<PersonInfo>({
    queryKey: ["person", id],
    queryFn: () =>
      eveyFetch<PersonInfo>({ url: `/api/v1/people/${id}`, method: "GET" }),
    enabled: !!id,
  });
}

export default function PersonDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const [bioExpanded, setBioExpanded] = useState(false);

  const personId = id ?? "";
  const { data: person, isLoading: personLoading } = useGetPerson(personId);
  const { data: mediaData, isLoading: mediaLoading } =
    useLibraryServiceListMediaItems(
      { personId, "pagination.pageSize": 200 },
      { query: { enabled: !!personId } },
    );

  const items = mediaData?.items ?? [];
  const name = person?.name ?? "Unknown";
  const initial = name[0]?.toUpperCase() ?? "?";
  const isLoading = personLoading || mediaLoading;

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ padding: PAD_H }}>
      {/* Header */}
      <View style={styles.headerRow}>
        <View style={[styles.avatar, { backgroundColor: theme.colors.surface }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 56, fontWeight: "bold" }}>{initial}</Text>
        </View>
        <View style={{ flex: 1, marginLeft: 32 }}>
          <Text style={[styles.name, { color: theme.colors.text }]}>{name}</Text>
          {person?.birthday ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 18, marginTop: 8 }}>
              {new Date(person.birthday).toLocaleDateString(undefined, {
                year: "numeric",
                month: "long",
                day: "numeric",
              })}
              {person.deathday
                ? ` \u2014 ${new Date(person.deathday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
                : ` (${Math.floor((Date.now() - new Date(person.birthday).getTime()) / 31557600000)} years old)`}
            </Text>
          ) : null}
          {person?.placeOfBirth ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: 4 }}>
              {person.placeOfBirth}
            </Text>
          ) : null}
        </View>
      </View>

      {/* Biography */}
      {person?.biography ? (
        <FocusableCard
          onPress={() => setBioExpanded((p) => !p)}
          style={{ marginTop: 24 }}
        >
          <Text
            style={{ color: theme.colors.text, fontSize: 18, lineHeight: 28 }}
            numberOfLines={bioExpanded ? undefined : 4}
          >
            {person.biography}
          </Text>
          <Text style={{ color: theme.colors.accent, fontSize: 16, marginTop: 8 }}>
            {bioExpanded ? "Show less" : "Read more"}
          </Text>
        </FocusableCard>
      ) : null}

      {/* Filmography */}
      {isLoading ? (
        <View style={{ paddingVertical: 40, alignItems: "center" }}>
          <ActivityIndicator size="large" color={theme.colors.accent} />
        </View>
      ) : null}

      {!isLoading && items.length > 0 ? (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
            Filmography ({items.length})
          </Text>
          <View style={styles.grid}>
            {items.map((item, index) => (
              <FocusableCard
                key={item.id}
                onPress={() => router.push(`/(auth)/media/${item.id}`)}
                hasTVPreferredFocus={index === 0}
                style={{ width: POSTER_WIDTH, marginBottom: POSTER_GAP }}
              >
                <View
                  style={[
                    styles.poster,
                    {
                      width: POSTER_WIDTH,
                      height: POSTER_HEIGHT,
                      backgroundColor: theme.colors.surface,
                      borderRadius: theme.radii.sm,
                    },
                  ]}
                />
                <Text
                  style={{ color: theme.colors.text, fontSize: 16, marginTop: 6, width: POSTER_WIDTH }}
                  numberOfLines={1}
                >
                  {item.title}
                </Text>
                {item.year ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 14, width: POSTER_WIDTH }}>
                    {item.year}
                  </Text>
                ) : null}
              </FocusableCard>
            ))}
          </View>
        </>
      ) : null}

      {!isLoading && items.length === 0 && !personLoading ? (
        <Text style={{ color: theme.colors.textMuted, textAlign: "center", paddingVertical: 40, fontSize: 20 }}>
          No filmography found
        </Text>
      ) : null}

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  headerRow: { flexDirection: "row", alignItems: "center" },
  avatar: { width: 140, height: 140, borderRadius: 70, justifyContent: "center", alignItems: "center" },
  name: { fontSize: 36, fontWeight: "bold" },
  sectionTitle: { fontSize: 28, fontWeight: "600", marginTop: 32, marginBottom: 16 },
  grid: { flexDirection: "row", flexWrap: "wrap", gap: POSTER_GAP },
  poster: { justifyContent: "center", alignItems: "center" },
});
