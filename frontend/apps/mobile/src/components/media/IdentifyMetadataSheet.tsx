import { useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, Modal, FlatList, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { eveyFetch, getMetadataServiceListMetadataOverridesQueryKey, getLibraryServiceGetMediaItemQueryKey } from "@evey/api";
import { toast } from "@evey/store";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { useTheme } from "../../providers/ThemeProvider";

interface IdentifyResult {
  tmdb_id?: string;
  imdb_id?: string;
  provider?: string;
  title: string;
  year?: number;
  overview: string;
  poster_url?: string;
}

interface Props {
  visible: boolean;
  onClose: () => void;
  mediaItemId: string;
  /** Pre-fills the search input. Usually the scan-parsed title. */
  initialQuery: string;
  mediaType: "movie" | "tv";
  onApplied?: () => void;
}

// Mirrors apps/web/src/features/library/IdentifyMetadataModal.tsx: same two
// admin endpoints (search-metadata, refresh-metadata), same tmdb-over-imdb
// preference when a result carries both.
export function IdentifyMetadataSheet({ visible, onClose, mediaItemId, initialQuery, mediaType, onApplied }: Props) {
  const { t } = useTranslation();
  const theme = useTheme();
  const qc = useQueryClient();
  const [query, setQuery] = useState(initialQuery);
  const [results, setResults] = useState<IdentifyResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [applyingId, setApplyingId] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  async function handleSearch() {
    const q = query.trim();
    if (q === "" || !mediaItemId) return;
    setSearching(true);
    try {
      const data = await eveyFetch<IdentifyResult[]>({
        url: `/api/v1/admin/media/${mediaItemId}/search-metadata`,
        method: "GET",
        params: { query: q, type: mediaType },
      });
      setResults(data ?? []);
      setSearched(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.identify.searchFailed", "Search failed")}: ${msg}`);
    } finally {
      setSearching(false);
    }
  }

  async function handleApply(result: IdentifyResult) {
    const applyId = result.tmdb_id || result.imdb_id;
    if (!applyId || !mediaItemId) return;
    setApplyingId(applyId);
    try {
      const payload: Record<string, string> = { media_type: mediaType };
      if (result.tmdb_id) payload.tmdb_id = result.tmdb_id;
      else if (result.imdb_id) payload.imdb_id = result.imdb_id;
      await eveyFetch<void>({
        url: `/api/v1/admin/media/${mediaItemId}/refresh-metadata`,
        method: "POST",
        data: payload,
      });
      void qc.invalidateQueries({ queryKey: getLibraryServiceGetMediaItemQueryKey(mediaItemId) });
      void qc.invalidateQueries({ queryKey: getMetadataServiceListMetadataOverridesQueryKey(mediaItemId) });
      toast.success(t("media.detail.identify.appliedTo", "Identified as {{title}}", { title: result.title }));
      onApplied?.();
      handleClose();
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(`${t("media.detail.identify.applyFailed", "Identify failed")}: ${msg}`);
    } finally {
      setApplyingId(null);
    }
  }

  function handleClose() {
    if (applyingId !== null) return;
    setResults([]);
    setSearched(false);
    setQuery(initialQuery);
    onClose();
  }

  const s = styles(theme);

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={handleClose}>
      <View style={[s.container, { backgroundColor: theme.colors.bg }]}>
        <View style={[s.header, { borderBottomColor: theme.colors.divider }]}>
          <Text style={[s.headerTitle, { color: theme.colors.text }]}>{t("media.detail.identify.title", "Identify on TMDB")}</Text>
          <Pressable onPress={handleClose} disabled={applyingId !== null} accessibilityRole="button" accessibilityLabel={t("common.close", "Close")}>
            <Ionicons name="close" size={ICON_SIZE.md} color={theme.colors.text} />
          </Pressable>
        </View>

        <View style={s.searchRow}>
          <TextInput
            value={query}
            onChangeText={setQuery}
            onSubmitEditing={() => void handleSearch()}
            placeholder={t("media.detail.identify.queryPlaceholder", "e.g. Greyhound")}
            placeholderTextColor={theme.colors.textMuted}
            accessibilityLabel={t("media.detail.identify.queryLabel", "Search title")}
            style={[s.input, { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}
          />
          <Pressable
            onPress={() => void handleSearch()}
            disabled={searching || !query.trim()}
            style={[s.searchBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: searching || !query.trim() ? 0.5 : 1 }]}
            accessibilityRole="button"
            accessibilityLabel={t("media.detail.identify.search", "Search")}
          >
            {searching ? (
              <ActivityIndicator color={theme.colors.onAccent} size="small" />
            ) : (
              <Text style={[s.searchBtnText, { color: theme.colors.onAccent }]}>{t("media.detail.identify.search", "Search")}</Text>
            )}
          </Pressable>
        </View>

        {searched && results.length === 0 && !searching && (
          <Text style={[s.noResults, { color: theme.colors.textMuted }]}>
            {t("media.detail.identify.noResults", "No TMDB matches. Try a shorter or differently spelled query.")}
          </Text>
        )}

        <FlatList
          data={results}
          keyExtractor={(r, idx) => `${r.tmdb_id ?? "noid"}-${idx}`}
          contentContainerStyle={s.list}
          renderItem={({ item: r }) => {
            const applyId = r.tmdb_id || r.imdb_id;
            const canApply = !!applyId && applyingId === null;
            return (
              <View style={[s.resultRow, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}>
                {r.poster_url ? (
                  <Image source={{ uri: r.poster_url }} style={s.poster} contentFit="cover" />
                ) : (
                  <View style={[s.poster, s.posterPlaceholder, { backgroundColor: theme.colors.surface }]}>
                    <Text style={{ color: theme.colors.textMuted }}>-</Text>
                  </View>
                )}
                <View style={s.resultInfo}>
                  <Text style={[s.resultTitle, { color: theme.colors.text }]} numberOfLines={2}>
                    {r.title}
                    {r.year != null && r.year > 0 ? ` (${r.year})` : ""}
                  </Text>
                  {r.overview ? (
                    <Text style={[s.resultOverview, { color: theme.colors.textMuted }]} numberOfLines={3}>
                      {r.overview}
                    </Text>
                  ) : null}
                  <Pressable
                    onPress={() => handleApply(r)}
                    disabled={!canApply}
                    style={[s.applyBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm, opacity: canApply ? 1 : 0.5 }]}
                    accessibilityRole="button"
                    accessibilityLabel={t("media.detail.identify.useThis", "Use this match")}
                  >
                    {applyingId === applyId ? (
                      <ActivityIndicator color={theme.colors.onAccent} size="small" />
                    ) : (
                      <Text style={[s.applyBtnText, { color: theme.colors.onAccent }]}>{t("media.detail.identify.useThis", "Use this match")}</Text>
                    )}
                  </Pressable>
                </View>
              </View>
            );
          }}
        />
      </View>
    </Modal>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: { flex: 1 },
    header: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingHorizontal: t.spacing.md,
      paddingVertical: t.spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
    },
    headerTitle: { fontSize: FONT_SIZE.lg, fontWeight: "700" },
    searchRow: { flexDirection: "row", gap: t.spacing.sm, paddingHorizontal: t.spacing.md, paddingTop: t.spacing.md },
    input: { flex: 1, borderWidth: 1, paddingHorizontal: 12, paddingVertical: 10, fontSize: FONT_SIZE.sm },
    searchBtn: { paddingHorizontal: 16, alignItems: "center", justifyContent: "center" },
    searchBtnText: { fontWeight: "600", fontSize: FONT_SIZE.sm },
    noResults: { fontSize: FONT_SIZE.sm, paddingHorizontal: t.spacing.md, paddingTop: t.spacing.md },
    list: { padding: t.spacing.md, gap: t.spacing.sm },
    resultRow: { flexDirection: "row", gap: t.spacing.sm, padding: t.spacing.sm, borderWidth: 1, marginBottom: t.spacing.sm },
    poster: { width: 64, height: 96, borderRadius: 4 },
    posterPlaceholder: { alignItems: "center", justifyContent: "center" },
    resultInfo: { flex: 1, gap: 4 },
    resultTitle: { fontSize: FONT_SIZE.sm, fontWeight: "600" },
    resultOverview: { fontSize: FONT_SIZE.xs },
    applyBtn: { alignSelf: "flex-start", paddingHorizontal: 12, paddingVertical: 6, marginTop: 4 },
    applyBtnText: { fontSize: FONT_SIZE.xs, fontWeight: "600" },
  });
