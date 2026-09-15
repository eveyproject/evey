import { memo, useCallback, useEffect, useMemo, useState } from "react";
import { View, Text, FlatList, TextInput, Pressable, ActivityIndicator, ScrollView, Share, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useInfiniteQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

// Mirrors frontend/apps/web/src/features/admin/logsClassify.ts. The search
// endpoint returns already-classified entries - this file only needs the
// shape and the filter vocabularies, not the client-side classifier (that
// only exists to classify the raw SSE live-tail lines, which mobile defers).
interface LogEntry {
  uid?: string;
  time: string;
  level: string;
  msg: string;
  component?: string;
  where: string;
  source: string;
  requestId?: string;
  traceId?: string;
  mediaId?: string;
  sessionId?: string;
  userId?: string;
  fields?: Record<string, unknown>;
  raw: string;
}

interface LogSearchResponse {
  entries: LogEntry[];
  total: number;
  scanned: number;
  truncated: boolean;
}

const LEVELS = ["ERROR", "WARN", "INFO", "DEBUG"] as const;
const WHERE_VALUES = [
  "player",
  "seek-index",
  "subtitle",
  "remux",
  "hw-detect",
  "intro",
  "trickplay",
  "qoe",
  "chromecast",
  "auth",
  "infra",
  "other",
] as const;
const SOURCE_VALUES = ["ffmpeg", "mkvgo", "qoe", "grpc", "client", "other"] as const;
const PAGE_SIZE = 50;

interface FilterParams {
  levels: string[];
  where: string;
  source: string;
  q: string;
  sessionId: string;
  userId: string;
  mediaId: string;
}

function buildSearchQS(f: FilterParams, limit: number, offset: number): string {
  const p = new URLSearchParams();
  if (f.levels.length) p.set("level", f.levels.join(","));
  if (f.where) p.set("where", f.where);
  if (f.source) p.set("source", f.source);
  if (f.q) p.set("q", f.q);
  if (f.sessionId) p.set("session_id", f.sessionId);
  if (f.userId) p.set("user_id", f.userId);
  if (f.mediaId) p.set("media_id", f.mediaId);
  // Successful gRPC access lines (heartbeats, polls) are four lines in five
  // on a live server and bury what the operator is looking for. Picking the
  // grpc source explicitly is the way to see them.
  if (f.source !== "grpc") p.set("hide_access", "1");
  p.set("limit", String(limit));
  p.set("offset", String(offset));
  return p.toString();
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(undefined, { hour12: false }) + "." + String(d.getMilliseconds()).padStart(3, "0");
}

function levelColor(theme: ReturnType<typeof useTheme>, level: string): string {
  switch (level) {
    case "ERROR":
      return theme.colors.error;
    case "WARN":
      return theme.colors.warning;
    case "INFO":
      return theme.colors.info;
    default:
      return theme.colors.textMuted;
  }
}

function Chip({
  label,
  active,
  onPress,
  theme,
}: {
  label: string;
  active: boolean;
  onPress: () => void;
  theme: ReturnType<typeof useTheme>;
}) {
  return (
    <Pressable
      onPress={onPress}
      style={[
        styles.chip,
        { backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md },
      ]}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>
        {label}
      </Text>
    </Pressable>
  );
}

const LogRow = memo(function LogRow({
  entry,
  theme,
  t,
}: {
  entry: LogEntry;
  theme: ReturnType<typeof useTheme>;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const [open, setOpen] = useState(false);

  const copy = useCallback(() => {
    void Share.share({ message: entry.raw });
  }, [entry.raw]);

  const ids: Array<[string, string | undefined]> = [
    ["media", entry.mediaId],
    ["session", entry.sessionId],
    ["user", entry.userId],
  ];

  return (
    <View style={[styles.row, { borderColor: theme.colors.border, borderLeftColor: levelColor(theme, entry.level), backgroundColor: theme.colors.surface }]}>
      <Pressable onPress={() => setOpen((v) => !v)} style={styles.rowHeader}>
        <Ionicons
          name={open ? "chevron-down" : "chevron-forward"}
          size={14}
          color={theme.colors.textMuted}
          style={{ marginTop: 2 }}
        />
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, fontFamily: "monospace" }}>{fmtTime(entry.time)}</Text>
        <View style={[styles.badge, { backgroundColor: levelColor(theme, entry.level) + "22" }]}>
          <Text style={{ color: levelColor(theme, entry.level), fontSize: 10, fontWeight: "700" }}>{entry.level || "?"}</Text>
        </View>
        <View style={[styles.badge, { backgroundColor: theme.colors.bg }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{entry.where}</Text>
        </View>
        <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1 }} numberOfLines={open ? undefined : 1}>
          {entry.msg}
        </Text>
      </Pressable>
      {open && (
        <View style={{ paddingLeft: 26, paddingRight: 8, paddingBottom: 10, gap: 6 }}>
          {(entry.component || ids.some(([, v]) => v) || entry.requestId) && (
            <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 10 }}>
              {entry.component ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>component: {entry.component}</Text>
              ) : null}
              {ids.map(([k, v]) =>
                v ? (
                  <Text key={k} style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {k}: {v}
                  </Text>
                ) : null,
              )}
              {entry.requestId ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>request_id: {entry.requestId}</Text>
              ) : null}
            </View>
          )}
          {entry.fields && Object.keys(entry.fields).length > 0 && (
            <View style={{ gap: 2 }}>
              {Object.entries(entry.fields).map(([k, v]) => (
                <Text key={k} style={{ color: theme.colors.text, fontSize: 11, fontFamily: "monospace" }}>
                  <Text style={{ color: theme.colors.accent }}>{k}</Text> {typeof v === "string" ? v : JSON.stringify(v)}
                </Text>
              ))}
            </View>
          )}
          <Pressable onPress={copy} style={{ flexDirection: "row", alignItems: "center", gap: 4, alignSelf: "flex-start" }}>
            <Ionicons name="copy-outline" size={13} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t("admin.logs.copyRaw")}</Text>
          </Pressable>
        </View>
      )}
    </View>
  );
});

export default function AdminLogsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();

  const [levels, setLevels] = useState<string[]>([]);
  const [where, setWhere] = useState("");
  const [source, setSource] = useState("");
  const [qInput, setQInput] = useState("");
  const [q, setQ] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [userId, setUserId] = useState("");
  const [mediaId, setMediaId] = useState("");
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Debounce the free-text box so we don't refetch on every keystroke.
  useEffect(() => {
    const id = setTimeout(() => setQ(qInput.trim()), 300);
    return () => clearTimeout(id);
  }, [qInput]);

  const filters: FilterParams = useMemo(
    () => ({ levels, where, source, q, sessionId: sessionId.trim(), userId: userId.trim(), mediaId: mediaId.trim() }),
    [levels, where, source, q, sessionId, userId, mediaId],
  );

  // Live tail: RN has no EventSource, so the newest page is polled instead of
  // streamed from /logs/stream - functionally the same auto-updating tail
  // (new lines land at offset 0). Off by default; only refetches loaded pages.
  const [live, setLive] = useState(false);

  const { data, isLoading, isRefetching, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ["admin-logs-search", filters],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      eveyFetch<LogSearchResponse>({
        url: `/api/v1/admin/logs/search?${buildSearchQS(filters, PAGE_SIZE, pageParam)}`,
        method: "GET",
        signal,
      }),
    getNextPageParam: (last, all) => {
      const fetched = all.reduce((n, p) => n + p.entries.length, 0);
      return fetched < last.total ? fetched : undefined;
    },
    refetchInterval: live ? 2500 : false,
    refetchIntervalInBackground: false,
  });

  const entries = useMemo(
    () =>
      data?.pages.flatMap((p, pageIdx) =>
        p.entries.map((e, rowIdx) => (e.uid ? e : { ...e, uid: `page-${pageIdx}-${rowIdx}` })),
      ) ?? [],
    [data],
  );
  const total = data?.pages[0]?.total ?? 0;
  const truncated = data?.pages[0]?.truncated ?? false;

  const toggleLevel = useCallback((l: string) => {
    setLevels((prev) => (prev.includes(l) ? prev.filter((x) => x !== l) : [...prev, l]));
  }, []);

  const loadMore = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(({ item }: { item: LogEntry }) => <LogRow entry={item} theme={theme} t={t} />, [theme, t]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("admin.logs.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      <FlatList
        data={entries}
        keyExtractor={(e, index) => e.uid ?? String(index)}
        renderItem={renderItem}
        onEndReached={loadMore}
        onEndReachedThreshold={0.5}
        refreshing={isLoading || isRefetching}
        onRefresh={refetch}
        contentContainerStyle={{ padding: 12, gap: 6 }}
        ListHeaderComponent={
          <View style={{ gap: 10, marginBottom: 10 }}>
            <View style={[styles.searchBox, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}>
              <Ionicons name="search-outline" size={16} color={theme.colors.textMuted} />
              <TextInput
                value={qInput}
                onChangeText={setQInput}
                placeholder={t("admin.logs.searchPlaceholder")}
                placeholderTextColor={theme.colors.textMuted}
                style={{ flex: 1, color: theme.colors.text, fontSize: 14, paddingVertical: 8 }}
                autoCapitalize="none"
                autoCorrect={false}
              />
            </View>

            <View style={{ gap: 6 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>{t("admin.logs.level.label")}</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                {LEVELS.map((l) => (
                  <Chip key={l} label={l} active={levels.includes(l)} onPress={() => toggleLevel(l)} theme={theme} />
                ))}
              </ScrollView>
            </View>

            <View style={{ gap: 6 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>{t("admin.logs.where.label")}</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                <Chip label={t("admin.logs.where.all")} active={where === ""} onPress={() => setWhere("")} theme={theme} />
                {WHERE_VALUES.map((w) => (
                  <Chip key={w} label={t(`admin.logs.where.${w}`)} active={where === w} onPress={() => setWhere(w)} theme={theme} />
                ))}
              </ScrollView>
            </View>

            <View style={{ gap: 6 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>{t("admin.logs.source.label")}</Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                <Chip label={t("admin.logs.source.all")} active={source === ""} onPress={() => setSource("")} theme={theme} />
                {SOURCE_VALUES.map((s) => (
                  <Chip key={s} label={t(`admin.logs.source.${s}`)} active={source === s} onPress={() => setSource(s)} theme={theme} />
                ))}
              </ScrollView>
            </View>

            <Pressable onPress={() => setShowAdvanced((v) => !v)} style={{ flexDirection: "row", alignItems: "center", gap: 6, alignSelf: "flex-start" }}>
              <Ionicons name="options-outline" size={14} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.logs.advanced")}</Text>
            </Pressable>

            {showAdvanced && (
              <View style={{ gap: 8 }}>
                {[
                  { label: t("admin.logs.sessionId"), value: sessionId, onChangeText: setSessionId, placeholder: "ps-..." },
                  { label: t("admin.logs.userId"), value: userId, onChangeText: setUserId, placeholder: "usr-..." },
                  { label: t("admin.logs.mediaId"), value: mediaId, onChangeText: setMediaId, placeholder: "mi-..." },
                ].map((f) => (
                  <View key={f.label} style={{ gap: 4 }}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{f.label}</Text>
                    <TextInput
                      value={f.value}
                      onChangeText={f.onChangeText}
                      placeholder={f.placeholder}
                      placeholderTextColor={theme.colors.textMuted}
                      autoCapitalize="none"
                      autoCorrect={false}
                      style={[
                        styles.input,
                        { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, borderRadius: theme.radii.sm, color: theme.colors.text },
                      ]}
                    />
                  </View>
                ))}
              </View>
            )}

            <Pressable
              onPress={() => setLive((v) => !v)}
              style={[styles.notice, { backgroundColor: live ? theme.colors.accent + "22" : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
              accessibilityRole="button"
              accessibilityState={{ selected: live }}
            >
              <Ionicons name={live ? "radio" : "radio-outline"} size={14} color={live ? theme.colors.accent : theme.colors.textMuted} />
              <Text style={{ color: live ? theme.colors.accent : theme.colors.textMuted, fontSize: 12, flex: 1, fontWeight: live ? "600" : "400" }}>
                {live ? t("admin.logs.liveOn", "Live tail on - newest lines auto-refresh") : t("admin.logs.live", "Live tail")}
              </Text>
              {live ? <ActivityIndicator size="small" color={theme.colors.accent} /> : null}
            </Pressable>

            <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.logs.resultCount", { count: total })}</Text>
              {truncated && (
                <View style={{ flexDirection: "row", alignItems: "center", gap: 4 }}>
                  <Ionicons name="alert-circle-outline" size={13} color={theme.colors.warning} />
                  <Text style={{ color: theme.colors.warning, fontSize: 11 }}>{t("admin.logs.truncated")}</Text>
                </View>
              )}
            </View>
          </View>
        }
        ListFooterComponent={isFetchingNextPage ? <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} /> : null}
        ListEmptyComponent={
          isLoading ? null : isError ? (
            <View style={{ alignItems: "center", paddingTop: 40 }}>
              <Ionicons name="alert-circle-outline" size={40} color={theme.colors.error} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 8 }}>{t("admin.logs.error")}</Text>
            </View>
          ) : (
            <View style={{ alignItems: "center", paddingTop: 40 }}>
              <Ionicons name="document-text-outline" size={40} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 8 }}>{t("admin.logs.empty")}</Text>
            </View>
          )
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { borderWidth: StyleSheet.hairlineWidth, borderLeftWidth: 3, overflow: "hidden" },
  rowHeader: { flexDirection: "row", alignItems: "flex-start", gap: 8, padding: 10 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  chip: { paddingHorizontal: 12, paddingVertical: 7 },
  searchBox: { flexDirection: "row", alignItems: "center", gap: 8, paddingHorizontal: 10, borderWidth: StyleSheet.hairlineWidth },
  input: { paddingHorizontal: 10, paddingVertical: 8, fontSize: 13, borderWidth: StyleSheet.hairlineWidth },
  notice: { flexDirection: "row", alignItems: "center", gap: 8, padding: 10 },
});
