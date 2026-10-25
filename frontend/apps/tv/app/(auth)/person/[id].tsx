import { useCallback, useEffect, useState } from "react";
import { View, Text, ScrollView, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { useLocalSearchParams, router } from "expo-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch, useLibraryServiceListMediaItems } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkPx, artworkUrl } from "../../../src/lib/artwork";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { ScreenState } from "../../../src/components/ScreenState";
import { TV_SCREEN_WIDTH, POSTER_RATIO, GRID_GAP } from "../../../src/lib/tvLayout";

const POSTER_COLS = 5;
const POSTER_GAP = GRID_GAP;
const PAD_H = 48;
// TV screens target a fixed 1920px-wide display - no runtime Dimensions lookup needed.
const POSTER_WIDTH = (TV_SCREEN_WIDTH - PAD_H * 2 - POSTER_GAP * (POSTER_COLS - 1)) / POSTER_COLS;
const FILMOGRAPHY_PAGE_SIZE = 24;
const POSTER_HEIGHT = POSTER_WIDTH * POSTER_RATIO;

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
  const { t } = useTranslation();
  const [bioExpanded, setBioExpanded] = useState(false);
  // A prolific actor's filmography used to mount up to 500 posters at once
  // (the fetch cap), freezing a modest TV box before first paint. The fetch
  // stays whole so the count in the heading stays honest; only the RENDER pages.
  const [visibleCount, setVisibleCount] = useState(FILMOGRAPHY_PAGE_SIZE);
  useEffect(() => {
    setVisibleCount(FILMOGRAPHY_PAGE_SIZE);
  }, [id]);

  const personId = id ?? "";
  const {
    data: person,
    isLoading: personLoading,
    isError: personError,
    refetch: refetchPerson,
  } = useGetPerson(personId);
  const {
    data: mediaData,
    isLoading: mediaLoading,
    isError: mediaError,
    refetch: refetchMedia,
  } = useLibraryServiceListMediaItems(
    // 500 (the server ceiling) rather than 200: the heading below shows
    // items.length as the filmography count, so a capped page turns into a
    // wrong TOTAL rather than a short list. No credited person approaches
    // 500 titles, whereas 200 is reachable.
    { personId, "pagination.pageSize": 500 },
    { query: { enabled: !!personId } },
  );
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();

  const items = mediaData?.items ?? [];
  const name = person?.name ?? t("common.unknown");
  const initial = name[0]?.toUpperCase() ?? "?";
  const isLoading = personLoading || mediaLoading;
  const isError = personError || mediaError;
  const refetch = useCallback(() => {
    if (personError) void refetchPerson();
    if (mediaError) void refetchMedia();
  }, [personError, mediaError, refetchPerson, refetchMedia]);

  // A failed person or filmography fetch used to leave the header on
  // "Unknown" with an empty grid below - unreadable as failure at three
  // metres, and no focusable target to retry from.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={refetch} />;
  }

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ padding: PAD_H }}>
      {/* Header */}
      <View style={styles.headerRow}>
        {person?.thumbUrl ? (
          <Image
            source={{ uri: person.thumbUrl }}
            style={styles.avatar}
            contentFit="cover"
          />
        ) : (
          <View style={[styles.avatar, { backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }]}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 56, fontWeight: "bold" }}>{initial}</Text>
          </View>
        )}
        <View style={{ flex: 1, marginLeft: 32 }}>
          <Text style={[styles.name, { color: theme.colors.text }]}>{name}</Text>
          {person?.birthday ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 17, marginTop: 8 }}>
              {new Date(person.birthday).toLocaleDateString(undefined, {
                year: "numeric",
                month: "long",
                day: "numeric",
              })}
              {person.deathday
                ? ` - ${new Date(person.deathday).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
                : ` (${Math.floor((Date.now() - new Date(person.birthday).getTime()) / 31557600000)} ${t("media.person.yearsOld")})`}
            </Text>
          ) : null}
          {person?.placeOfBirth ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 4 }}>
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
          accessibilityLabel={bioExpanded ? t("common.showLess") : t("common.readMore")}
        >
          <Text
            style={{ color: theme.colors.text, fontSize: 17, lineHeight: 28 }}
            numberOfLines={bioExpanded ? undefined : 4}
          >
            {person.biography}
          </Text>
          <Text style={{ color: theme.colors.accent, fontSize: 15, marginTop: 8 }}>
            {bioExpanded ? t("common.showLess") : t("common.readMore")}
          </Text>
        </FocusableCard>
      ) : null}

      {/* Filmography - the top-level guard above already covers loading/error. */}
      {items.length > 0 ? (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
            {t("media.person.filmography")} ({items.length})
          </Text>
          <View style={styles.grid}>
            {items.slice(0, visibleCount).map((item, index) => (
              <FocusableCard
                key={item.id}
                onPress={() => router.push(`/(auth)/media/${item.id}`)}
                hasTVPreferredFocus={index === 0}
                style={{ width: POSTER_WIDTH, marginBottom: POSTER_GAP }}
                accessibilityLabel={item.title}
              >
                {serverUrl && item.id ? (
                  <Image
                    source={{ uri: artworkUrl(serverUrl, item.id, "poster", artworkPx(POSTER_WIDTH), item), headers: authHeaders }}
                    style={{
                      width: POSTER_WIDTH,
                      height: POSTER_HEIGHT,
                      backgroundColor: theme.colors.surface,
                      borderRadius: theme.radii.sm,
                    }}
                    contentFit="cover"
                    recyclingKey={item.id}
                  />
                ) : (
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
                )}
                <Text
                  style={{ color: theme.colors.text, fontSize: 15, marginTop: 6, width: POSTER_WIDTH }}
                  numberOfLines={1}
                >
                  {item.title}
                </Text>
                {item.year ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 13, width: POSTER_WIDTH }}>
                    {item.year}
                  </Text>
                ) : null}
              </FocusableCard>
            ))}
            {items.length > visibleCount ? (
              <FocusableCard
                onPress={() => setVisibleCount((c) => c + FILMOGRAPHY_PAGE_SIZE)}
                style={{ width: POSTER_WIDTH, marginBottom: POSTER_GAP, justifyContent: "center", alignItems: "center", minHeight: 120 }}
                accessibilityLabel={t("media.detail.showMoreEpisodes", "Show more")}
              >
                <Text style={{ color: theme.colors.accent, fontSize: 17, fontWeight: "600" }}>
                  {t("media.detail.showMoreEpisodes", "Show more")}
                </Text>
              </FocusableCard>
            ) : null}
          </View>
        </>
      ) : null}

      {items.length === 0 ? (
        <Text style={{ color: theme.colors.textMuted, textAlign: "center", paddingVertical: 40, fontSize: 18 }}>
          {t("media.person.noFilmography")}
        </Text>
      ) : null}

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  headerRow: { flexDirection: "row", alignItems: "center" },
  avatar: { width: 140, height: 140, borderRadius: 70, justifyContent: "center", alignItems: "center" },
  name: { fontSize: 30, fontWeight: "bold" },
  sectionTitle: { fontSize: 24, fontWeight: "600", marginTop: 32, marginBottom: 16 },
  grid: { flexDirection: "row", flexWrap: "wrap", gap: POSTER_GAP },
  poster: { justifyContent: "center", alignItems: "center" },
});
