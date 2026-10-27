import { useMemo, useState, useCallback } from "react";
import { View, Text, FlatList, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useMediaRequestServiceGetRequestPolicy,
  useMediaRequestServiceSearchTitles,
  useMediaRequestServiceCreateRequest,
  useMediaRequestServiceListMyRequests,
  getMediaRequestServiceListMyRequestsQueryKey,
  type V1TitleCandidate,
  type V1MediaRequestProto,
} from "@evey/api";
import { toast, useDemoStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { ScreenState } from "../../../src/components/ScreenState";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { RequestFacts, RequestOverview } from "../../../src/components/requests/RequestDetails";

type MediaType = "movie" | "tv" | "music";
// The search scopes offered to the user: movies and series are ONE scope
// (two provider queries merged client-side), matching web/mobile - music
// stays its own scope.
type SearchScope = "video" | "music";
type Tab = "search" | "mine";

const SCOPES: { key: SearchScope; labelKey: string; icon: keyof typeof Ionicons.glyphMap }[] = [
  { key: "video", labelKey: "requests.typeVideo", icon: "film-outline" },
  { key: "music", labelKey: "requests.typeMusic", icon: "musical-notes-outline" },
];

// A search candidate tagged with the concrete request type it will be
// submitted as - needed once movies and series share one result list.
type TaggedCandidate = V1TitleCandidate & { requestType: MediaType };

// interleave merges two ranked lists preserving each one's own order
// (movie[0], tv[0], movie[1], tv[1], ...) so neither type buries the other.
// Mirrors mobile's app/(auth)/(tabs)/requests.tsx.
function interleave<T>(a: T[], b: T[]): T[] {
  const out: T[] = [];
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if (i < a.length) out.push(a[i]);
    if (i < b.length) out.push(b[i]);
  }
  return out;
}

function statusColor(theme: ReturnType<typeof useTheme>, status?: string) {
  switch (status) {
    case "approved": return theme.colors.success;
    case "rejected": return theme.colors.danger;
    case "later": return theme.colors.warning;
    default: return theme.colors.textMuted;
  }
}

// candidateKey includes the media type: with movies and series in one list,
// a TMDB movie id and a TMDB tv id can collide numerically.
function candidateKey(c: { mediaType?: string; provider?: string; providerId?: string }) {
  return `${c.mediaType ?? ""}:${c.provider ?? ""}:${c.providerId ?? ""}`;
}

function capitalize(s?: string) {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function TabButton({
  label,
  active,
  onPress,
  hasTVPreferredFocus,
}: {
  label: string;
  active: boolean;
  onPress: () => void;
  hasTVPreferredFocus?: boolean;
}) {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      hasTVPreferredFocus={hasTVPreferredFocus}
      accessible={true}
      accessibilityRole="tab"
      accessibilityLabel={label}
      accessibilityState={{ selected: active }}
      style={[
        styles.tabBtn,
        { borderBottomColor: active ? theme.colors.accent : "transparent" },
        focused && { backgroundColor: theme.colors.accent + "22" },
      ]}
    >
      <Text style={{ color: active ? theme.colors.accent : theme.colors.textMuted, fontSize: 20, fontWeight: "600" }}>
        {label}
      </Text>
    </Pressable>
  );
}

function ScopeChip({
  label,
  icon,
  active,
  onPress,
}: {
  label: string;
  icon: keyof typeof Ionicons.glyphMap;
  active: boolean;
  onPress: () => void;
}) {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      accessible={true}
      accessibilityRole="radio"
      accessibilityLabel={label}
      accessibilityState={{ selected: active }}
      style={[
        styles.chip,
        { backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised },
        focused && { borderColor: theme.colors.accent },
      ]}
    >
      <Ionicons name={icon} size={18} color={active ? theme.colors.onAccent : theme.colors.text} />
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 17, marginLeft: 8 }}>
        {label}
      </Text>
    </Pressable>
  );
}

export default function RequestsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [chosenTab, setTab] = useState<Tab>("search");
  // The demo refuses the title search (it spends the operator's TMDB key) and
  // the request itself: only the list of existing requests is shown there.
  const demo = useDemoStore((s) => s.demoMode);
  const tab: Tab = demo ? "mine" : chosenTab;
  const [scope, setScope] = useState<SearchScope>("video");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");

  const {
    data: policy,
    isLoading: policyLoading,
    isError: policyError,
    refetch: refetchPolicy,
  } = useMediaRequestServiceGetRequestPolicy();
  const canRequest = policy?.canRequest === true;

  const { data: myRequestsResp, isLoading: mineLoading } = useMediaRequestServiceListMyRequests(
    undefined,
    { query: { enabled: canRequest } },
  );
  const myRequests = useMemo(() => myRequestsResp?.items ?? [], [myRequestsResp?.items]);
  const requestedKeys = useMemo(() => new Set(myRequests.map((r) => candidateKey(r))), [myRequests]);

  // Video scope fires BOTH provider searches and merges them; the server's
  // SearchTitles contract stays single-type. Music keeps its own single query.
  const searchEnabled = canRequest && submittedQuery.trim().length > 0;
  const movieQ = useMediaRequestServiceSearchTitles(
    { mediaType: "movie", query: submittedQuery },
    { query: { enabled: searchEnabled && scope === "video" } },
  );
  const tvQ = useMediaRequestServiceSearchTitles(
    { mediaType: "tv", query: submittedQuery },
    { query: { enabled: searchEnabled && scope === "video" } },
  );
  const musicQ = useMediaRequestServiceSearchTitles(
    { mediaType: "music", query: submittedQuery },
    { query: { enabled: searchEnabled && scope === "music" } },
  );

  const searching = scope === "video" ? movieQ.isFetching || tvQ.isFetching : musicQ.isFetching;

  const results: TaggedCandidate[] = useMemo(() => {
    if (scope === "music") {
      return (musicQ.data?.results ?? []).map((r) => ({ ...r, requestType: "music" as const }));
    }
    return interleave<TaggedCandidate>(
      (movieQ.data?.results ?? []).map((r) => ({ ...r, requestType: "movie" as const })),
      (tvQ.data?.results ?? []).map((r) => ({ ...r, requestType: "tv" as const })),
    );
  }, [scope, movieQ.data, tvQ.data, musicQ.data]);

  const createRequest = useMediaRequestServiceCreateRequest({
    mutation: {
      onSuccess: () => {
        toast.success(t("requests.requestSubmitted"));
        void queryClient.invalidateQueries({ queryKey: getMediaRequestServiceListMyRequestsQueryKey() });
      },
      onError: () => toast.error(t("requests.requestFailed")),
    },
  });

  const handleRequest = useCallback((candidate: TaggedCandidate) => {
    createRequest.mutate({
      data: {
        mediaType: candidate.requestType,
        provider: candidate.provider,
        providerId: candidate.providerId,
        title: candidate.title,
        year: candidate.year,
        posterUrl: candidate.posterUrl,
        // What the requester saw, kept with the request for the operator.
        overview: candidate.overview,
        releaseDate: candidate.releaseDate,
        rating: candidate.rating,
      },
    });
  }, [createRequest]);

  const renderResultItem = useCallback(({ item, index }: { item: TaggedCandidate; index: number }) => {
    const already = requestedKeys.has(candidateKey({ ...item, mediaType: item.requestType }));
    const inLibrary = item.inLibrary === true;
    return (
      <View style={styles.resultRow}>
        <View style={[styles.thumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised, overflow: "hidden" }]}>
          {item.posterUrl ? (
            <Image cachePolicy="memory-disk" source={{ uri: item.posterUrl }} style={StyleSheet.absoluteFill} contentFit="cover" />
          ) : (
            <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>?</Text>
          )}
          {inLibrary && (
            <View style={[styles.inLibraryBadge, { backgroundColor: theme.colors.overlay }]}>
              <Ionicons name="checkmark-circle" size={16} color={theme.colors.text} />
            </View>
          )}
        </View>
        <View style={{ flex: 1, marginLeft: 20 }}>
          <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "600" }} numberOfLines={2}>
            {item.title}{item.year != null && item.year > 0 ? ` (${item.year})` : ""}
          </Text>
          {scope === "video" && (
            <View style={[styles.typeBadge, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
              <Ionicons
                name={item.requestType === "tv" ? "tv-outline" : "film-outline"}
                size={14}
                color={theme.colors.textMuted}
              />
              <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginLeft: 6 }}>
                {t(item.requestType === "tv" ? "requests.typeSeries" : "requests.typeMovie")}
              </Text>
            </View>
          )}
          <RequestFacts releaseDate={item.releaseDate} rating={item.rating} />
          {inLibrary && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 4 }}>
              {t("requests.inLibrary")}
            </Text>
          )}
          <RequestOverview overview={item.overview} lines={2} />
        </View>
        {inLibrary && item.mediaItemId ? (
          <FocusableCard
            onPress={() => router.push(`/media/${item.mediaItemId}`)}
            hasTVPreferredFocus={index === 0}
            accessibilityLabel={`${t("requests.watchNow")} ${item.title ?? ""}`}
            style={[styles.actionBtn, { backgroundColor: theme.colors.surfaceRaised }]}
          >
            <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }}>
              {t("requests.watchNow")}
            </Text>
          </FocusableCard>
        ) : (
          <FocusableCard
            onPress={() => handleRequest(item)}
            hasTVPreferredFocus={index === 0}
            accessibilityLabel={`${already ? t("requests.requested") : t("requests.request")} ${item.title ?? ""}`}
            disabled={already || createRequest.isPending}
            style={[
              styles.actionBtn,
              { backgroundColor: already ? theme.colors.surfaceRaised : theme.colors.accent },
            ]}
          >
            <Text style={{ color: already ? theme.colors.textMuted : theme.colors.onAccent, fontSize: 17, fontWeight: "600" }}>
              {already ? t("requests.requested") : t("requests.request")}
            </Text>
          </FocusableCard>
        )}
      </View>
    );
  }, [requestedKeys, scope, theme, t, router, handleRequest, createRequest.isPending]);

  const renderMineItem = useCallback(({ item, index }: { item: V1MediaRequestProto; index: number }) => (
    <FocusableCard
      hasTVPreferredFocus={index === 0}
      accessibilityLabel={item.title ?? t("requests.title")}
      style={[styles.resultRow, { backgroundColor: "transparent" }]}
    >
      <View style={[styles.thumb, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised, overflow: "hidden" }]}>
        {item.posterUrl ? (
          <Image cachePolicy="memory-disk" source={{ uri: item.posterUrl }} style={StyleSheet.absoluteFill} contentFit="cover" />
        ) : (
          <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>?</Text>
        )}
      </View>
      <View style={{ flex: 1, marginLeft: 20 }}>
        <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "600" }} numberOfLines={2}>
          {item.title}{item.year != null && item.year > 0 ? ` (${item.year})` : ""}
        </Text>
        <RequestFacts releaseDate={item.releaseDate} rating={item.rating} />
        <Text style={{ color: statusColor(theme, item.status), fontSize: 17, marginTop: 4, fontWeight: "600" }}>
          {t(`requests.status${capitalize(item.status)}`)}
        </Text>
        <RequestOverview overview={item.overview} lines={2} />
        {item.adminNote ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 4 }}>
            {t("requests.adminNote", { note: item.adminNote })}
          </Text>
        ) : null}
      </View>
    </FocusableCard>
  ), [theme, t]);

  // A failed policy fetch used to land on the "requests are disabled" screen
  // below, because an absent policy and a forbidding one are indistinguishable
  // once the error is dropped. Telling the viewer a feature is off when the
  // server merely did not answer is the worse of the two mistakes.
  if (policyLoading || policyError) {
    return <ScreenState loading={policyLoading} error={policyError} onRetry={() => void refetchPolicy()} />;
  }

  if (!canRequest) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Ionicons name="mail-unread-outline" size={64} color={theme.colors.textMuted} />
        <Text style={[styles.emptyTitle, { color: theme.colors.text }]}>{t("requests.disabledTitle")}</Text>
        <Text style={[styles.emptyDesc, { color: theme.colors.textMuted }]}>{t("requests.disabledDesc")}</Text>
        {/* Focusable target for the disabled state - without it the remote has
            nowhere to go and gets stuck on this screen. */}
        <FocusableCard
          onPress={() => router.push("/(auth)/(tabs)/library")}
          hasTVPreferredFocus
          accessibilityLabel={t("media.library.libraries")}
          style={[styles.emptyAction, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{t("media.library.libraries")}</Text>
        </FocusableCard>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("requests.title")}</Text>

      <View style={styles.tabRow}>
        {demo ? null : (
          <TabButton
            label={t("requests.tabSearch")}
            active={tab === "search"}
            onPress={() => setTab("search")}
            hasTVPreferredFocus
          />
        )}
        <TabButton
          label={t("requests.tabMine")}
          active={tab === "mine"}
          onPress={() => setTab("mine")}
          hasTVPreferredFocus={demo}
        />
      </View>

      {tab === "search" ? (
        <View style={{ flex: 1 }}>
          <View style={styles.chipRow}>
            {SCOPES.map((sc) => (
              <ScopeChip
                key={sc.key}
                label={t(sc.labelKey)}
                icon={sc.icon}
                active={scope === sc.key}
                onPress={() => { setScope(sc.key); setSubmittedQuery(""); }}
              />
            ))}
          </View>

          <TextInput
            style={[
              styles.searchInput,
              { backgroundColor: theme.colors.surface, color: theme.colors.text, borderRadius: theme.radii.md },
            ]}
            placeholder={t("requests.searchPlaceholder")}
            placeholderTextColor={theme.colors.textMuted}
            value={query}
            onChangeText={setQuery}
            onSubmitEditing={() => setSubmittedQuery(query.trim())}
            returnKeyType="search"
            autoCapitalize="none"
            autoCorrect={false}
            accessible={true}
            accessibilityLabel={t("requests.searchPlaceholder")}
          />

          {searching ? (
            <View style={styles.center}>
              <ActivityIndicator color={theme.colors.accent} size="large" />
            </View>
          ) : submittedQuery === "" ? (
            <View style={styles.center}>
              <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.searchPrompt")}</Text>
            </View>
          ) : results.length === 0 ? (
            <View style={styles.center}>
              <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.noResults")}</Text>
            </View>
          ) : (
            <FlatList
              data={results}
              keyExtractor={(item) => candidateKey({ ...item, mediaType: item.requestType })}
              contentContainerStyle={{ paddingHorizontal: 24, paddingBottom: 48 }}
              ItemSeparatorComponent={() => <View style={{ height: 8 }} />}
              renderItem={renderResultItem}
              initialNumToRender={6}
              maxToRenderPerBatch={10}
              windowSize={5}
              removeClippedSubviews={true}
            />
          )}
        </View>
      ) : (
        <FlatList
          data={myRequests}
          keyExtractor={(item: V1MediaRequestProto) => item.id ?? ""}
          contentContainerStyle={{ paddingHorizontal: 24, paddingTop: 8, paddingBottom: 48 }}
          ItemSeparatorComponent={() => <View style={{ height: 8 }} />}
          renderItem={renderMineItem}
          ListEmptyComponent={
            mineLoading ? (
              <View style={styles.center}>
                <ActivityIndicator color={theme.colors.accent} size="large" />
              </View>
            ) : (
              <View style={styles.center}>
                <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.noRequests")}</Text>
              </View>
            )
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingHorizontal: 48 },
  header: { fontSize: 30, fontWeight: "700", paddingHorizontal: 24, paddingTop: 32 },
  emptyTitle: { fontSize: 21, fontWeight: "600", marginTop: 16, textAlign: "center" },
  emptyDesc: { fontSize: 17, marginTop: 8, textAlign: "center" },
  emptyAction: { marginTop: 24, paddingVertical: 10, paddingHorizontal: 28 },
  tabRow: { flexDirection: "row", paddingHorizontal: 24, marginTop: 16, gap: 16 },
  tabBtn: { paddingBottom: 10, paddingHorizontal: 4, borderBottomWidth: 3 },
  chipRow: { flexDirection: "row", paddingHorizontal: 24, marginTop: 16, gap: 10 },
  chip: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 24,
    borderWidth: 2,
    borderColor: "transparent",
  },
  searchInput: {
    marginHorizontal: 24,
    marginTop: 16,
    paddingHorizontal: 20,
    paddingVertical: 10,
    fontSize: 20,
  },
  hint: { textAlign: "center", fontSize: 18 },
  resultRow: { flexDirection: "row", alignItems: "center", paddingVertical: 9 },
  // Same ratio as web/mobile; a size that still reads from the couch now that
  // the row carries the date, the rating and the synopsis (2026-10-07).
  thumb: { width: 96, height: 144, justifyContent: "center", alignItems: "center" },
  inLibraryBadge: {
    position: "absolute",
    bottom: 4,
    right: 4,
    width: 24,
    height: 24,
    borderRadius: 12,
    alignItems: "center",
    justifyContent: "center",
  },
  typeBadge: { flexDirection: "row", alignItems: "center", alignSelf: "flex-start", paddingHorizontal: 8, paddingVertical: 4, marginTop: 6 },
  actionBtn: { paddingHorizontal: 20, paddingVertical: 10, borderRadius: 8 },
});
