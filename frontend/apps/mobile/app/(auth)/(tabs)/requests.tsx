import { useMemo, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TextInput,
  TouchableOpacity,
  ScrollView,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
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
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { Skeleton } from "../../../src/components/Skeleton";
import { LoadFailed } from "../../../src/components/LoadFailed";

type MediaType = "movie" | "tv" | "music";
// The search scopes offered to the user: movies and series are ONE scope
// (two provider queries merged client-side), matching the web app - music
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

// Poster with a title + media-type fallback (web parity: RequestPoster). A bare
// surface rectangle for a poster-less title left the row looking broken; the
// icon + title says what it is at a glance.
function RequestPoster({ posterUrl, title, mediaType }: { posterUrl?: string; title?: string; mediaType?: string }) {
  const theme = useTheme();
  if (posterUrl) {
    return <Image cachePolicy="memory-disk" source={{ uri: posterUrl }} style={{ width: 48, height: 72, borderRadius: theme.radii.sm }} contentFit="cover" />;
  }
  const icon: keyof typeof Ionicons.glyphMap =
    mediaType === "music" ? "musical-notes-outline" : mediaType === "tv" ? "tv-outline" : "film-outline";
  return (
    <View style={{ width: 48, height: 72, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised, alignItems: "center", justifyContent: "center", paddingHorizontal: 3 }}>
      <Ionicons name={icon} size={18} color={theme.colors.textMuted} />
      <Text numberOfLines={2} style={{ color: theme.colors.textMuted, fontSize: 8, textAlign: "center", marginTop: 2 }}>{title}</Text>
    </View>
  );
}

// interleave merges two ranked lists preserving each one's own order
// (movie[0], tv[0], movie[1], tv[1], ...) so neither type buries the other.
function interleave<T>(a: T[], b: T[]): T[] {
  const out: T[] = [];
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if (i < a.length) out.push(a[i]);
    if (i < b.length) out.push(b[i]);
  }
  return out;
}

const statusColor = (theme: ReturnType<typeof useTheme>, status?: string) => {
  switch (status) {
    case "approved": return theme.colors.success;
    case "rejected": return theme.colors.danger;
    case "later": return theme.colors.warning;
    default: return theme.colors.textMuted;
  }
};

// candidateKey includes the media type: with movies and series in one list,
// a TMDB movie id and a TMDB tv id can collide numerically.
function candidateKey(c: { mediaType?: string; provider?: string; providerId?: string }) {
  return `${c.mediaType ?? ""}:${c.provider ?? ""}:${c.providerId ?? ""}`;
}

function capitalize(s?: string) {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export default function RequestsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("search");
  const [scope, setScope] = useState<SearchScope>("video");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");

  const { data: policy, isLoading: policyLoading, isError: policyError, refetch: refetchPolicy } = useMediaRequestServiceGetRequestPolicy();
  const canRequest = policy?.canRequest === true;

  const { data: myRequestsResp, isLoading: mineLoading, isError: mineError, refetch: refetchMine } = useMediaRequestServiceListMyRequests(
    undefined,
    { query: { enabled: canRequest } },
  );
  const myRequests = myRequestsResp?.items ?? [];
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
  const searchFailed = scope === "video" ? movieQ.isError || tvQ.isError : musicQ.isError;
  const retrySearch = () => {
    if (scope === "video") {
      void movieQ.refetch();
      void tvQ.refetch();
    } else {
      void musicQ.refetch();
    }
  };

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

  function handleRequest(candidate: TaggedCandidate) {
    createRequest.mutate({
      data: {
        mediaType: candidate.requestType,
        provider: candidate.provider,
        providerId: candidate.providerId,
        title: candidate.title,
        year: candidate.year,
        posterUrl: candidate.posterUrl,
      },
    });
  }

  if (policyLoading) {
    return (
      <View style={[styles.container, styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  // A failed policy fetch must not read as "requests are disabled" - that is
  // an admin decision, not a network hiccup, and the two need different next
  // steps from the user (retry vs. give up).
  if (policyError) {
    return (
      <View style={[styles.container, styles.center, { backgroundColor: theme.colors.bg }]}>
        <LoadFailed onRetry={() => void refetchPolicy()} />
      </View>
    );
  }

  if (!canRequest) {
    return (
      <View style={[styles.container, styles.center, { backgroundColor: theme.colors.bg }]}>
        <Ionicons name="mail-unread-outline" size={48} color={theme.colors.textMuted} />
        <Text style={[styles.emptyTitle, { color: theme.colors.text }]}>{t("requests.disabledTitle")}</Text>
        <Text style={[styles.emptyDesc, { color: theme.colors.textMuted }]}>{t("requests.disabledDesc")}</Text>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Text style={[styles.header, { color: theme.colors.text, marginTop: theme.spacing.xl }]}>
        {t("requests.title")}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 2, marginBottom: 4 }}>
        {t("requests.subtitle")}
      </Text>

      <View style={styles.tabRow}>
        {(["search", "mine"] as Tab[]).map((tb) => (
          <TouchableOpacity
            key={tb}
            style={[
              styles.tabBtn,
              tab === tb && { borderBottomColor: theme.colors.accent, borderBottomWidth: 2 },
            ]}
            onPress={() => setTab(tb)}
            accessibilityRole="tab"
            accessibilityState={{ selected: tab === tb }}
          >
            <Text style={{ color: tab === tb ? theme.colors.accent : theme.colors.textMuted, fontSize: 14, fontWeight: "600" }}>
              {t(tb === "search" ? "requests.tabSearch" : "requests.tabMine")}
            </Text>
          </TouchableOpacity>
        ))}
      </View>

      {tab === "search" ? (
        <View style={{ flex: 1 }}>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.chipRow}>
            {SCOPES.map((sc) => (
              <TouchableOpacity
                key={sc.key}
                style={[
                  styles.chip,
                  { backgroundColor: scope === sc.key ? theme.colors.accent : theme.colors.surfaceRaised },
                ]}
                onPress={() => { setScope(sc.key); setSubmittedQuery(""); }}
                accessibilityRole="radio"
                accessibilityState={{ selected: scope === sc.key }}
              >
                <Ionicons name={sc.icon} size={14} color={scope === sc.key ? theme.colors.onAccent : theme.colors.text} />
                <Text style={{ color: scope === sc.key ? theme.colors.onAccent : theme.colors.text, fontSize: 13, marginLeft: 4 }}>
                  {t(sc.labelKey)}
                </Text>
              </TouchableOpacity>
            ))}
          </ScrollView>

          <TextInput
            style={[
              styles.searchInput,
              { backgroundColor: theme.colors.surface, color: theme.colors.text, borderRadius: theme.radii.sm },
            ]}
            placeholder={t("requests.searchPlaceholder")}
            placeholderTextColor={theme.colors.textMuted}
            value={query}
            onChangeText={setQuery}
            onSubmitEditing={() => setSubmittedQuery(query.trim())}
            returnKeyType="search"
            autoCapitalize="none"
            autoCorrect={false}
          />

          {searching ? (
            <View style={{ paddingTop: 8 }}>
              {Array.from({ length: 4 }).map((_, i) => (
                <View key={i} style={styles.resultRow}>
                  <Skeleton width={48} height={72} borderRadius={theme.radii.sm} />
                </View>
              ))}
            </View>
          ) : submittedQuery === "" ? (
            <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.searchPrompt")}</Text>
          ) : searchFailed ? (
            <LoadFailed inline onRetry={retrySearch} />
          ) : results.length === 0 ? (
            <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.noResults")}</Text>
          ) : (
            <FlatList
              data={results}
              keyExtractor={(item) => candidateKey({ ...item, mediaType: item.requestType })}
              contentContainerStyle={{ paddingBottom: 24 }}
              renderItem={({ item }) => {
                const already = requestedKeys.has(candidateKey({ ...item, mediaType: item.requestType }));
                const inLibrary = item.inLibrary === true;
                return (
                  <View style={styles.resultRow}>
                    <View style={{ width: 48, height: 72 }}>
                      <RequestPoster posterUrl={item.posterUrl} title={item.title} mediaType={item.requestType} />
                      {inLibrary && (
                        <View style={[styles.inLibraryBadge, { backgroundColor: theme.colors.overlay }]}>
                          <Ionicons name="checkmark-circle" size={10} color={theme.colors.text} />
                        </View>
                      )}
                    </View>
                    <View style={{ flex: 1, marginLeft: 10 }}>
                      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={2}>
                        {item.title}{item.year != null && item.year > 0 ? ` (${item.year})` : ""}
                      </Text>
                      {scope === "video" && (
                        <View
                          style={[
                            styles.typeBadge,
                            { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm },
                          ]}
                        >
                          <Ionicons
                            name={item.requestType === "tv" ? "tv-outline" : "film-outline"}
                            size={11}
                            color={theme.colors.textMuted}
                          />
                          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginLeft: 4 }}>
                            {t(item.requestType === "tv" ? "requests.typeSeries" : "requests.typeMovie")}
                          </Text>
                        </View>
                      )}
                      {inLibrary && (
                        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
                          {t("requests.inLibrary")}
                        </Text>
                      )}
                    </View>
                    {inLibrary && item.mediaItemId ? (
                      <TouchableOpacity
                        style={[styles.requestBtn, { backgroundColor: theme.colors.surfaceRaised }]}
                        onPress={() => router.push(`/media/${item.mediaItemId}`)}
                        accessibilityRole="button"
                      >
                        <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>
                          {t("requests.watchNow")}
                        </Text>
                      </TouchableOpacity>
                    ) : (
                      <TouchableOpacity
                        style={[
                          styles.requestBtn,
                          { backgroundColor: already ? theme.colors.surfaceRaised : theme.colors.accent },
                        ]}
                        disabled={already || createRequest.isPending}
                        onPress={() => handleRequest(item)}
                        accessibilityRole="button"
                      >
                        <Text style={{ color: already ? theme.colors.textMuted : theme.colors.onAccent, fontSize: 12, fontWeight: "600" }}>
                          {already ? t("requests.requested") : t("requests.request")}
                        </Text>
                      </TouchableOpacity>
                    )}
                  </View>
                );
              }}
            />
          )}
        </View>
      ) : mineLoading ? (
        <View style={{ paddingTop: 8 }}>
          {Array.from({ length: 4 }).map((_, i) => (
            <View key={i} style={styles.resultRow}>
              <Skeleton width={48} height={72} borderRadius={theme.radii.sm} />
            </View>
          ))}
        </View>
      ) : mineError && myRequests.length === 0 ? (
        <LoadFailed onRetry={() => void refetchMine()} />
      ) : (
        <FlatList
          data={myRequests}
          keyExtractor={(item: V1MediaRequestProto) => item.id ?? ""}
          contentContainerStyle={{ paddingBottom: 24, paddingTop: 8 }}
          refreshing={mineLoading}
          renderItem={({ item }) => (
            <View style={styles.resultRow}>
              <RequestPoster posterUrl={item.posterUrl} title={item.title} mediaType={item.mediaType} />
              <View style={{ flex: 1, marginLeft: 10 }}>
                <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={2}>
                  {item.title}{item.year != null && item.year > 0 ? ` (${item.year})` : ""}
                </Text>
                <Text style={{ color: statusColor(theme, item.status), fontSize: 12, marginTop: 2, fontWeight: "600" }}>
                  {t(`requests.status${capitalize(item.status)}`)}
                  {/* answeredAt has been served since media requests shipped and
                      no client ever showed it: a denied request gave no clue how
                      old the decision was. Locale-formatted, no new i18n key -
                      the status word right before it carries the wording. */}
                  {item.answeredAt
                    ? ` · ${new Date(item.answeredAt).toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" })}`
                    : ""}
                </Text>
                {item.adminNote ? (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
                    {t("requests.adminNote", { note: item.adminNote })}
                  </Text>
                ) : null}
              </View>
            </View>
          )}
          ListEmptyComponent={
            <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.noRequests")}</Text>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: { justifyContent: "center", alignItems: "center", paddingHorizontal: 32 },
  header: { fontSize: 28, fontWeight: "700", paddingHorizontal: 16 },
  emptyTitle: { fontSize: 16, fontWeight: "600", marginTop: 12, textAlign: "center" },
  emptyDesc: { fontSize: 13, marginTop: 4, textAlign: "center" },
  tabRow: { flexDirection: "row", paddingHorizontal: 16, marginTop: 12, gap: 16 },
  tabBtn: { paddingBottom: 8 },
  // flexShrink 0: the results list below can outgrow the screen, and Yoga
  // would otherwise squeeze the chip row until it is clipped.
  chipRow: { paddingHorizontal: 16, marginTop: 12, flexGrow: 0, flexShrink: 0 },
  chip: { flexDirection: "row", alignItems: "center", paddingHorizontal: 12, paddingVertical: 6, borderRadius: 20, marginRight: 8 },
  searchInput: { marginHorizontal: 16, marginTop: 10, paddingHorizontal: 12, paddingVertical: 8, fontSize: 14 },
  hint: { textAlign: "center", fontSize: 13, marginTop: 24, paddingHorizontal: 16 },
  resultRow: { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 8 },
  typeBadge: { flexDirection: "row", alignItems: "center", alignSelf: "flex-start", paddingHorizontal: 6, paddingVertical: 2, marginTop: 4 },
  requestBtn: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: 8 },
  inLibraryBadge: {
    position: "absolute",
    bottom: 2,
    right: 2,
    width: 16,
    height: 16,
    borderRadius: 8,
    alignItems: "center",
    justifyContent: "center",
  },
});
