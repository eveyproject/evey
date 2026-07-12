import { useMemo, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TextInput,
  TouchableOpacity,
  ScrollView,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
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

type MediaType = "movie" | "tv" | "music";
type Tab = "search" | "mine";

const TYPES: { key: MediaType; labelKey: string; icon: keyof typeof Ionicons.glyphMap }[] = [
  { key: "movie", labelKey: "requests.typeMovie", icon: "film-outline" },
  { key: "tv", labelKey: "requests.typeSeries", icon: "tv-outline" },
  { key: "music", labelKey: "requests.typeMusic", icon: "musical-notes-outline" },
];

const statusColor = (theme: ReturnType<typeof useTheme>, status?: string) => {
  switch (status) {
    case "approved": return theme.colors.success;
    case "rejected": return theme.colors.danger;
    case "later": return theme.colors.warning;
    default: return theme.colors.textMuted;
  }
};

function candidateKey(c: { provider?: string; providerId?: string }) {
  return `${c.provider ?? ""}:${c.providerId ?? ""}`;
}

function capitalize(s?: string) {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export default function RequestsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<Tab>("search");
  const [mediaType, setMediaType] = useState<MediaType>("movie");
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");

  const { data: policy, isLoading: policyLoading } = useMediaRequestServiceGetRequestPolicy();
  const canRequest = policy?.canRequest === true;

  const { data: myRequestsResp, isLoading: mineLoading } = useMediaRequestServiceListMyRequests(
    undefined,
    { query: { enabled: canRequest } },
  );
  const myRequests = myRequestsResp?.items ?? [];
  const requestedKeys = useMemo(() => new Set(myRequests.map((r) => candidateKey(r))), [myRequests]);

  const { data: searchData, isFetching: searching } = useMediaRequestServiceSearchTitles(
    { mediaType, query: submittedQuery },
    { query: { enabled: canRequest && submittedQuery.trim().length > 0 } },
  );
  const results = searchData?.results ?? [];

  const createRequest = useMediaRequestServiceCreateRequest({
    mutation: {
      onSuccess: () => {
        toast.success(t("requests.requestSubmitted"));
        void queryClient.invalidateQueries({ queryKey: getMediaRequestServiceListMyRequestsQueryKey() });
      },
      onError: () => toast.error(t("requests.requestFailed")),
    },
  });

  function handleRequest(candidate: V1TitleCandidate) {
    createRequest.mutate({
      data: {
        mediaType,
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
      <View style={[styles.container, styles.center, { backgroundColor: theme.colors.bg }]} />
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
            {TYPES.map((tp) => (
              <TouchableOpacity
                key={tp.key}
                style={[
                  styles.chip,
                  { backgroundColor: mediaType === tp.key ? theme.colors.accent : theme.colors.surfaceRaised },
                ]}
                onPress={() => { setMediaType(tp.key); setSubmittedQuery(""); }}
                accessibilityRole="radio"
                accessibilityState={{ selected: mediaType === tp.key }}
              >
                <Ionicons name={tp.icon} size={14} color={mediaType === tp.key ? theme.colors.onAccent : theme.colors.text} />
                <Text style={{ color: mediaType === tp.key ? theme.colors.onAccent : theme.colors.text, fontSize: 13, marginLeft: 4 }}>
                  {t(tp.labelKey)}
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
          ) : results.length === 0 ? (
            <Text style={[styles.hint, { color: theme.colors.textMuted }]}>{t("requests.noResults")}</Text>
          ) : (
            <FlatList
              data={results}
              keyExtractor={candidateKey}
              contentContainerStyle={{ paddingBottom: 24 }}
              renderItem={({ item }) => {
                const already = requestedKeys.has(candidateKey(item));
                return (
                  <View style={styles.resultRow}>
                    {item.posterUrl ? (
                      <Image source={{ uri: item.posterUrl }} style={{ width: 48, height: 72, borderRadius: theme.radii.sm }} contentFit="cover" />
                    ) : (
                      <View style={{ width: 48, height: 72, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised }} />
                    )}
                    <View style={{ flex: 1, marginLeft: 10 }}>
                      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={2}>
                        {item.title}{item.year != null && item.year > 0 ? ` (${item.year})` : ""}
                      </Text>
                    </View>
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
                  </View>
                );
              }}
            />
          )}
        </View>
      ) : (
        <FlatList
          data={myRequests}
          keyExtractor={(item: V1MediaRequestProto) => item.id ?? ""}
          contentContainerStyle={{ paddingBottom: 24, paddingTop: 8 }}
          refreshing={mineLoading}
          renderItem={({ item }) => (
            <View style={styles.resultRow}>
              {item.posterUrl ? (
                <Image source={{ uri: item.posterUrl }} style={{ width: 48, height: 72, borderRadius: theme.radii.sm }} contentFit="cover" />
              ) : (
                <View style={{ width: 48, height: 72, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised }} />
              )}
              <View style={{ flex: 1, marginLeft: 10 }}>
                <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={2}>
                  {item.title}{item.year != null && item.year > 0 ? ` (${item.year})` : ""}
                </Text>
                <Text style={{ color: statusColor(theme, item.status), fontSize: 12, marginTop: 2, fontWeight: "600" }}>
                  {t(`requests.status${capitalize(item.status)}`)}
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
  chipRow: { paddingHorizontal: 16, marginTop: 12, flexGrow: 0 },
  chip: { flexDirection: "row", alignItems: "center", paddingHorizontal: 12, paddingVertical: 6, borderRadius: 20, marginRight: 8 },
  searchInput: { marginHorizontal: 16, marginTop: 10, paddingHorizontal: 12, paddingVertical: 8, fontSize: 14 },
  hint: { textAlign: "center", fontSize: 13, marginTop: 24, paddingHorizontal: 16 },
  resultRow: { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 8 },
  requestBtn: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: 8 },
});
