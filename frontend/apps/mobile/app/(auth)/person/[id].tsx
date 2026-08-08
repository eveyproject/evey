import { useState } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Dimensions,
  ActivityIndicator,
} from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, router, Stack } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch, personDepartmentKey, useLibraryServiceListMediaItems } from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { LoadFailed } from "../../../src/components/LoadFailed";

const { width: SCREEN_WIDTH } = Dimensions.get("window");
const POSTER_COLS = 3;
const POSTER_GAP = 8;

interface PersonInfo {
  id?: string;
  name?: string;
  thumbUrl?: string;
  biography?: string;
  birthday?: string;
  deathday?: string;
  placeOfBirth?: string;
  knownFor?: string;
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
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [bioExpanded, setBioExpanded] = useState(false);

  const personId = id ?? "";
  const { data: person, isLoading: personLoading } = useGetPerson(personId);
  const { data: mediaData, isLoading: mediaLoading, isError: mediaError, refetch: refetchMedia } =
    useLibraryServiceListMediaItems(
      // 500 (the server ceiling) rather than 200: the heading below shows
      // items.length as the filmography count, so a capped page turns into a
      // wrong TOTAL rather than a short list. No credited person approaches
      // 500 titles, whereas 200 is reachable.
      { personId, "pagination.pageSize": 500 },
      { query: { enabled: !!personId } },
    );

  const items = mediaData?.items ?? [];
  const name = person?.name ?? "Unknown";
  const initial = name[0]?.toUpperCase() ?? "?";
  const isLoading = personLoading || mediaLoading;

  const padH = theme.spacing.md;
  const posterWidth = (SCREEN_WIDTH - padH * 2 - POSTER_GAP * (POSTER_COLS - 1)) / POSTER_COLS;
  const posterHeight = posterWidth * 1.5;

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: name,
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {/* Header */}
      <View
        style={{
          alignItems: "center",
          paddingTop: theme.spacing.xl,
          paddingHorizontal: padH,
        }}
      >
        {person?.thumbUrl ? (
          <Image
            source={{ uri: person.thumbUrl }}
            style={{
              width: 120,
              height: 120,
              borderRadius: 60,
              backgroundColor: theme.colors.surface,
            }}
            contentFit="cover"
            transition={200}
          />
        ) : (
          <View
            style={{
              width: 120,
              height: 120,
              borderRadius: 60,
              backgroundColor: theme.colors.surface,
              justifyContent: "center",
              alignItems: "center",
            }}
          >
            <Text style={{ color: theme.colors.textMuted, fontSize: 40, fontWeight: "bold" }}>
              {initial}
            </Text>
          </View>
        )}

        <Text
          style={{
            color: theme.colors.text,
            fontSize: 24,
            fontWeight: "bold",
            marginTop: theme.spacing.md,
            textAlign: "center",
          }}
        >
          {name}
        </Text>

        {/* knownFor is TMDB's known_for_department, stored in English.
            Localize the closed set; an unknown value renders raw. */}
        {person?.knownFor ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.xs, textAlign: "center" }}>
            {(() => {
              const key = personDepartmentKey(person.knownFor);
              return key ? t(key, person.knownFor) : person.knownFor;
            })()}
          </Text>
        ) : null}

        {/* Birth / death info */}
        {person?.birthday ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.xs, textAlign: "center" }}>
            {new Date(person.birthday).toLocaleDateString(undefined, {
              year: "numeric",
              month: "long",
              day: "numeric",
            })}
            {person.deathday
              ? ` - ${new Date(person.deathday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
              : ` (${Math.floor((Date.now() - new Date(person.birthday).getTime()) / 31557600000)} years old)`}
          </Text>
        ) : null}

        {person?.placeOfBirth ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2, textAlign: "center" }}>
            {person.placeOfBirth}
          </Text>
        ) : null}
      </View>

      {/* Biography */}
      {person?.biography ? (
        <Pressable
          onPress={() => setBioExpanded((p) => !p)}
          style={{ paddingHorizontal: padH, marginTop: theme.spacing.md }}
        >
          <Text
            style={{ color: theme.colors.text, fontSize: 14, lineHeight: 20 }}
            numberOfLines={bioExpanded ? undefined : 4}
          >
            {person.biography}
          </Text>
          <Text style={{ color: theme.colors.accent, fontSize: 13, marginTop: 4 }}>
            {bioExpanded ? t("common.showLess") : t("common.readMore")}
          </Text>
        </Pressable>
      ) : null}

      {/* Filmography count */}
      {!isLoading && items.length > 0 ? (
        <Text
          style={{
            color: theme.colors.text,
            fontSize: 16,
            fontWeight: "600",
            paddingHorizontal: padH,
            marginTop: theme.spacing.lg,
            marginBottom: theme.spacing.sm,
          }}
        >
          {t("media.person.filmography")} ({items.length})
        </Text>
      ) : null}

      {/* Loading */}
      {isLoading ? (
        <View style={{ paddingVertical: 40, alignItems: "center" }}>
          <ActivityIndicator size="large" color={theme.colors.accent} />
        </View>
      ) : null}

      {/* Poster grid */}
      {!isLoading && items.length > 0 ? (
        <View
          style={{
            flexDirection: "row",
            flexWrap: "wrap",
            paddingHorizontal: padH,
            gap: POSTER_GAP,
          }}
        >
          {items.map((item) => (
            <PosterCard
              key={item.id}
              item={item}
              width={posterWidth}
              height={posterHeight}
              serverUrl={serverUrl}
              authHeaders={authHeaders}
              theme={theme}
            />
          ))}
        </View>
      ) : null}

      {!isLoading && mediaError ? (
        <LoadFailed inline onRetry={() => void refetchMedia()} />
      ) : !isLoading && items.length === 0 && !personLoading ? (
        <Text
          style={{
            color: theme.colors.textMuted,
            textAlign: "center",
            paddingVertical: 40,
            fontSize: 14,
          }}
        >
          {t("media.person.noFilmography")}
        </Text>
      ) : null}

      <View style={{ height: theme.spacing.xl * 2 }} />
    </ScrollView>
  );
}

function PosterCard({
  item,
  width,
  height,
  serverUrl,
  authHeaders,
  theme,
}: {
  item: V1MediaItem;
  width: number;
  height: number;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  theme: ReturnType<typeof useTheme>;
}) {
  const uri = item.id ? artworkUrl(serverUrl, item.id, "poster", 200) : undefined;
  return (
    <Pressable onPress={() => router.push(`/(auth)/media/${item.id}`)}>
      <Image
        source={uri ? { uri, headers: authHeaders } : undefined}
        style={{
          width,
          height,
          borderRadius: theme.radii.sm,
          backgroundColor: theme.colors.surface,
        }}
        contentFit="cover"
        transition={200}
      />
      <Text
        style={{
          color: theme.colors.text,
          fontSize: 12,
          marginTop: 4,
          width,
        }}
        numberOfLines={1}
      >
        {item.title}
      </Text>
      {item.year ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11, width }}>
          {item.year}
        </Text>
      ) : null}
    </Pressable>
  );
}
