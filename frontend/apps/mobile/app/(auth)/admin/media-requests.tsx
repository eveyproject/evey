import { useMemo, useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Modal, Switch, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import {
  adminMediaRequestServiceListRequests,
  useAdminMediaRequestServiceAnswerRequest,
  useAdminMediaRequestServiceGetAccessSettings,
  useAdminMediaRequestServiceSetAccessDefault,
  useAdminMediaRequestServiceSetUserAccess,
  getAdminMediaRequestServiceListRequestsQueryKey,
  getAdminMediaRequestServiceGetAccessSettingsQueryKey,
  useUserServiceListUsers,
  V1AccessMode,
} from "@evey/api";
import type { V1MediaRequestProto, V1UserAccessOverride } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { capitalizeStatus, requestStatusI18nKey, STATUS_FILTERS, type StatusFilter } from "../../../src/components/admin/mediaRequestsHelpers";
import { RequestFacts, RequestOverview, RequestTitleLink, useExternalTitleLink } from "../../../src/components/requests/RequestDetails";

type Tab = "queue" | "access";

// Mirrors the web's status->i18n key mapping (requests.status*) so the
// mobile queue reads identically to the request page users already see.
function statusLabel(t: ReturnType<typeof useTranslation>["t"], status?: string): string {
  return t(requestStatusI18nKey(status));
}

function statusColor(theme: ReturnType<typeof useTheme>, status?: string): string {
  switch (status) {
    case "approved":
      return theme.colors.success;
    case "rejected":
      return theme.colors.error;
    case "later":
      return theme.colors.warning;
    default:
      return theme.colors.accent;
  }
}

// posterUrl comes straight from a metadata provider (TMDB / MusicBrainz),
// not our server - no serverUrl prefix or auth token needed, and it can 404
// (e.g. a MusicBrainz release-group with no cover art), so fall back to a
// type icon on error. Mirrors web's RequestPoster.
function RequestPosterThumb({ request, theme }: { request: V1MediaRequestProto; theme: ReturnType<typeof useTheme> }) {
  const [failed, setFailed] = useState(false);
  const ext = useExternalTitleLink(request);
  const mediaType = request.mediaType;
  const iconName = mediaType === "music" ? "musical-notes" : mediaType === "tv" ? "tv-outline" : "film-outline";

  const body = !request.posterUrl || failed ? (
    <View
      style={[
        styles.posterThumb,
        { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm, alignItems: "center", justifyContent: "center" },
      ]}
      testID="request-poster-fallback"
    >
      <Ionicons name={iconName} size={24} color={theme.colors.textMuted} />
    </View>
  ) : (
    <Image
      source={{ uri: request.posterUrl }}
      style={[styles.posterThumb, { borderRadius: theme.radii.sm }]}
      contentFit="cover"
      onError={() => setFailed(true)}
    />
  );
  if (!ext.url) return body;
  return (
    <Pressable onPress={ext.open} accessibilityRole="link" accessibilityLabel={ext.label}>
      {body}
    </Pressable>
  );
}

function RequestRow({ request, theme }: { request: V1MediaRequestProto; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [note, setNote] = useState("");

  const answer = useAdminMediaRequestServiceAnswerRequest({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: getAdminMediaRequestServiceListRequestsQueryKey() });
      },
      onError: (_err, vars) => {
        const key = vars.data.status === "approved" ? "approveFailed" : vars.data.status === "rejected" ? "rejectFailed" : "laterFailed";
        toast.error(t(`admin.mediaRequests.${key}`));
      },
    },
  });

  function handleAnswer(status: "approved" | "rejected" | "later") {
    if (!request.id) return;
    answer.mutate({ id: request.id, data: { status, note: note.trim() || undefined } });
  }

  const isPending = request.status === "pending";

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, flexDirection: "row", gap: 12 }]}>
      <RequestPosterThumb request={request} theme={theme} />
      <View style={{ flex: 1 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
          <RequestTitleLink title={request} text={request.title} year={request.year} size={15} />
          <View style={[styles.badge, { backgroundColor: statusColor(theme, request.status) + "22" }]}>
            <Text style={{ color: statusColor(theme, request.status), fontSize: 11, fontWeight: "600" }}>
              {statusLabel(t, request.status)}
            </Text>
          </View>
        </View>
        <RequestFacts releaseDate={request.releaseDate} rating={request.rating} />
        <RequestOverview overview={request.overview} lines={4} />
        {request.requesterName ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 4 }}>
            {t("admin.mediaRequests.requestedBy", { name: request.requesterName })}
          </Text>
        ) : null}

        {isPending ? (
          <View style={{ marginTop: 10, gap: 8 }}>
            <TextInput
              value={note}
              onChangeText={setNote}
              placeholder={t("admin.mediaRequests.notePlaceholder")}
              placeholderTextColor={theme.colors.textMuted}
              multiline
              style={[styles.noteInput, { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.sm }]}
            />
            <View style={{ flexDirection: "row", gap: 8 }}>
              <Pressable
                disabled={answer.isPending}
                onPress={() => handleAnswer("approved")}
                style={[styles.actionBtn, { backgroundColor: theme.colors.accent, opacity: answer.isPending ? 0.6 : 1 }]}
                accessible={true}
                accessibilityLabel={t("admin.mediaRequests.approveNamed", "Approve {{title}}", { title: request.title })}
                accessibilityRole="button"
              >
                <Text style={{ color: theme.colors.onAccent, fontSize: 13, fontWeight: "600" }}>{t("admin.mediaRequests.approve")}</Text>
              </Pressable>
              <Pressable
                disabled={answer.isPending}
                onPress={() => handleAnswer("rejected")}
                style={[styles.actionBtn, { backgroundColor: theme.colors.error, opacity: answer.isPending ? 0.6 : 1 }]}
                accessible={true}
                accessibilityLabel={t("admin.mediaRequests.rejectNamed", "Reject {{title}}", { title: request.title })}
                accessibilityRole="button"
              >
                <Text style={{ color: theme.colors.onError, fontSize: 13, fontWeight: "600" }}>{t("admin.mediaRequests.reject")}</Text>
              </Pressable>
              <Pressable
                disabled={answer.isPending}
                onPress={() => handleAnswer("later")}
                style={[styles.actionBtn, { backgroundColor: theme.colors.bg, opacity: answer.isPending ? 0.6 : 1 }]}
                accessible={true}
                accessibilityLabel={t("admin.mediaRequests.laterNamed", "Postpone {{title}}", { title: request.title })}
                accessibilityRole="button"
              >
                <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t("admin.mediaRequests.later")}</Text>
              </Pressable>
            </View>
          </View>
        ) : (
          request.adminNote ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 6 }}>
              {t("admin.mediaRequests.answeredNote", { note: request.adminNote })}
            </Text>
          ) : null
        )}
      </View>
    </View>
  );
}

const QUEUE_PAGE_SIZE = 50;

function QueueTab({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<StatusFilter>("pending");

  // Pages of QUEUE_PAGE_SIZE loaded as the list is scrolled: the queue used to
  // ask for nothing and get the server's default 100, past which older
  // requests were silently gone. The key keeps the generated prefix so the
  // answer mutation's invalidation still refreshes it.
  const filterParams = filter === "all" ? {} : { statuses: [filter] };
  const { data, isLoading, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: [...getAdminMediaRequestServiceListRequestsQueryKey(filterParams), "pages"],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      adminMediaRequestServiceListRequests({ ...filterParams, limit: QUEUE_PAGE_SIZE, offset: pageParam }, signal),
    getNextPageParam: (last, _pages, lastOffset) =>
      (last.items?.length ?? 0) < QUEUE_PAGE_SIZE ? undefined : lastOffset + QUEUE_PAGE_SIZE,
  });
  const items = useMemo(() => data?.pages.flatMap((p) => p.items ?? []) ?? [], [data]);
  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  return (
    <FlatList
      data={items}
      keyExtractor={(item, index) => item.id ?? String(index)}
      renderItem={({ item }) => <RequestRow request={item} theme={theme} />}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      refreshing={isLoading}
      onRefresh={refetch}
      onEndReached={handleEndReached}
      onEndReachedThreshold={0.5}
      ListFooterComponent={isFetchingNextPage ? <ActivityIndicator style={{ marginVertical: 12 }} color={theme.colors.accent} /> : null}
      ListHeaderComponent={
        <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginBottom: 12 }}>
          {STATUS_FILTERS.map((f) => (
            <Pressable
              key={f}
              onPress={() => setFilter(f)}
              style={[
                styles.filterChip,
                { backgroundColor: filter === f ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md },
              ]}
              accessible={true}
              accessibilityLabel={t(`admin.mediaRequests.filter${capitalizeStatus(f)}`)}
              accessibilityRole="button"
              accessibilityState={{ selected: filter === f }}
            >
              <Text style={{ color: filter === f ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
                {t(`admin.mediaRequests.filter${capitalizeStatus(f)}`)}
              </Text>
            </Pressable>
          ))}
        </View>
      }
      ListEmptyComponent={
        isLoading ? null : isError ? (
          <LoadFailed onRetry={() => void refetch()} />
        ) : (
          <View style={{ alignItems: "center", paddingTop: 40 }}>
            <Ionicons name="mail-open-outline" size={48} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.mediaRequests.empty")}</Text>
          </View>
        )
      }
    />
  );
}

function AddOverrideModal({
  visible,
  onClose,
  theme,
  availableUsers,
  onPick,
  pending,
}: {
  visible: boolean;
  onClose: () => void;
  theme: ReturnType<typeof useTheme>;
  availableUsers: Array<{ id?: string; username?: string }>;
  onPick: (userId: string, mode: V1AccessMode) => void;
  pending: boolean;
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<V1AccessMode>(V1AccessMode.ACCESS_MODE_ENABLED);

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <Pressable
        style={styles.backdrop}
        onPress={onClose}
        accessibilityElementsHidden={true}
        importantForAccessibility="no-hide-descendants"
      />
      <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
        <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 12 }}>
          {t("admin.mediaRequests.addOverride")}
        </Text>
        <View style={{ flexDirection: "row", gap: 8, marginBottom: 12 }}>
          {[
            { value: V1AccessMode.ACCESS_MODE_ENABLED, label: t("admin.mediaRequests.accessModeEnabled") },
            { value: V1AccessMode.ACCESS_MODE_DISABLED, label: t("admin.mediaRequests.accessModeDisabled") },
          ].map((opt) => (
            <Pressable
              key={opt.value}
              onPress={() => setMode(opt.value)}
              style={[
                styles.modeChip,
                { backgroundColor: mode === opt.value ? theme.colors.accent : theme.colors.bg, borderRadius: theme.radii.sm },
              ]}
              accessible={true}
              accessibilityLabel={opt.label}
              accessibilityRole="radio"
              accessibilityState={{ selected: mode === opt.value }}
            >
              <Text style={{ color: mode === opt.value ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                {opt.label}
              </Text>
            </Pressable>
          ))}
        </View>
        <View style={{ maxHeight: 320 }}>
          {availableUsers.length === 0 ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 13, paddingVertical: 12 }}>
              {t("admin.mediaRequests.selectUser")}
            </Text>
          ) : (
            <FlatList
              data={availableUsers}
              keyExtractor={(u) => u.id ?? ""}
              renderItem={({ item }) => (
                <Pressable
                  disabled={pending}
                  onPress={() => item.id && onPick(item.id, mode)}
                  style={[styles.userRow, { borderBottomColor: theme.colors.border }]}
                  accessible={true}
                  accessibilityLabel={item.username ?? item.id}
                  accessibilityRole="button"
                >
                  <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 8 }} numberOfLines={1}>
                    {item.username ?? item.id}
                  </Text>
                  <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
                </Pressable>
              )}
            />
          )}
        </View>
        <Pressable
          onPress={onClose}
          style={{ paddingVertical: 12, alignItems: "center" }}
          accessible={true}
          accessibilityLabel={t("common.cancel")}
          accessibilityRole="button"
        >
          <Text style={{ color: theme.colors.accent, fontWeight: "600", fontSize: 15 }}>{t("common.cancel")}</Text>
        </Pressable>
      </View>
    </Modal>
  );
}

function AccessTab({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: settings, isLoading, isError, refetch } = useAdminMediaRequestServiceGetAccessSettings();
  const { data: usersData } = useUserServiceListUsers();
  const [showAdd, setShowAdd] = useState(false);

  const setDefault = useAdminMediaRequestServiceSetAccessDefault({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.mediaRequests.accessDefaultUpdated"));
        void queryClient.invalidateQueries({ queryKey: getAdminMediaRequestServiceGetAccessSettingsQueryKey() });
      },
      onError: () => toast.error(t("admin.mediaRequests.accessDefaultFailed")),
    },
  });

  const setUserAccess = useAdminMediaRequestServiceSetUserAccess({
    mutation: {
      onSuccess: (_data, vars) => {
        const name = usersData?.users?.find((u) => u.id === vars.userId)?.username ?? vars.userId;
        toast.success(t("admin.mediaRequests.accessOverrideUpdated", { name }));
        void queryClient.invalidateQueries({ queryKey: getAdminMediaRequestServiceGetAccessSettingsQueryKey() });
        setShowAdd(false);
      },
      onError: () => toast.error(t("admin.mediaRequests.accessOverrideFailed")),
    },
  });

  const overrides = useMemo(() => settings?.overrides ?? [], [settings?.overrides]);
  const overriddenIds = useMemo(() => new Set(overrides.map((o) => o.userId)), [overrides]);
  const availableUsers = (usersData?.users ?? []).filter((u) => u.id && !overriddenIds.has(u.id));

  const handlePick = useCallback((userId: string, mode: V1AccessMode) => {
    setUserAccess.mutate({ userId, data: { mode } });
  }, [setUserAccess]);

  if (isLoading) {
    return (
      <View style={{ alignItems: "center", paddingTop: 60 }}>
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  if (isError) {
    return <LoadFailed onRetry={() => void refetch()} />;
  }

  return (
    <FlatList
      data={overrides}
      keyExtractor={(o) => o.userId ?? ""}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      ListHeaderComponent={
        <>
          <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginBottom: 16, flexDirection: "row", alignItems: "center", gap: 12 }]}>
            <View style={{ flex: 1 }}>
              <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>
                {t("admin.mediaRequests.accessDefaultLabel")}
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
                {t("admin.mediaRequests.accessDefaultDesc")}
              </Text>
            </View>
            <Switch
              value={settings?.defaultEnabled ?? false}
              onValueChange={() => setDefault.mutate({ data: { enabled: !(settings?.defaultEnabled ?? false) } })}
              disabled={setDefault.isPending}
              trackColor={{ true: theme.colors.accent }}
            />
          </View>

          <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
            <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "700" }}>
              {t("admin.mediaRequests.accessOverridesTitle")}
            </Text>
            {availableUsers.length > 0 && (
              <Pressable
                onPress={() => setShowAdd(true)}
                hitSlop={8}
                accessible={true}
                accessibilityLabel={t("admin.mediaRequests.addOverride")}
                accessibilityRole="button"
              >
                <Ionicons name="add-circle-outline" size={24} color={theme.colors.accent} />
              </Pressable>
            )}
          </View>

          {overrides.length === 0 && (
            <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: 8 }}>
              {t("admin.mediaRequests.noOverrides")}
            </Text>
          )}
        </>
      }
      renderItem={({ item }) => (
        <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, flexDirection: "row", alignItems: "center", gap: 12 }]}>
          <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>{item.username}</Text>
          <View style={{ flexDirection: "row", gap: 6 }}>
            {[
              { value: V1AccessMode.ACCESS_MODE_ENABLED, label: t("admin.mediaRequests.accessModeEnabled") },
              { value: V1AccessMode.ACCESS_MODE_DISABLED, label: t("admin.mediaRequests.accessModeDisabled") },
              { value: V1AccessMode.ACCESS_MODE_DEFAULT, label: t("admin.mediaRequests.accessModeDefault") },
            ].map((opt) => {
              const active =
                (opt.value === V1AccessMode.ACCESS_MODE_ENABLED && item.enabled) ||
                (opt.value === V1AccessMode.ACCESS_MODE_DISABLED && !item.enabled);
              return (
                <Pressable
                  key={opt.value}
                  disabled={setUserAccess.isPending}
                  onPress={() => item.userId && setUserAccess.mutate({ userId: item.userId, data: { mode: opt.value } })}
                  style={[
                    styles.modeChip,
                    { backgroundColor: active ? theme.colors.accent : theme.colors.bg, borderRadius: theme.radii.sm },
                  ]}
                  accessible={true}
                  accessibilityLabel={`${opt.label} - ${item.username}`}
                  accessibilityRole="radio"
                  accessibilityState={{ selected: active }}
                >
                  <Text style={{ color: active ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>
                    {opt.label}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      )}
      ListFooterComponent={
        <AddOverrideModal
          visible={showAdd}
          onClose={() => setShowAdd(false)}
          theme={theme}
          availableUsers={availableUsers as V1UserAccessOverride[]}
          onPick={handlePick}
          pending={setUserAccess.isPending}
        />
      }
    />
  );
}

export default function AdminMediaRequestsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("queue");

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "queue", label: t("admin.mediaRequests.tabQueue") },
    { id: "access", label: t("admin.mediaRequests.tabAccess") },
  ];

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.mediaRequests.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      <View style={[styles.tabBar, { backgroundColor: theme.colors.surface, borderBottomColor: theme.colors.border }]}>
        {tabs.map(({ id, label }) => (
          <Pressable
            key={id}
            onPress={() => setTab(id)}
            style={[
              styles.tabBtn,
              { backgroundColor: tab === id ? theme.colors.accent : "transparent", borderRadius: theme.radii.sm },
            ]}
            accessible={true}
            accessibilityLabel={label}
            accessibilityRole="tab"
            accessibilityState={{ selected: tab === id }}
          >
            <Text style={{ color: tab === id ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
              {label}
            </Text>
          </Pressable>
        ))}
      </View>

      {tab === "queue" ? <QueueTab theme={theme} /> : <AccessTab theme={theme} />}
    </View>
  );
}

const styles = StyleSheet.create({
  tabBar: { flexDirection: "row", padding: 8, gap: 4, borderBottomWidth: StyleSheet.hairlineWidth },
  tabBtn: { flex: 1, paddingVertical: 8, alignItems: "center" },
  card: { padding: 14 },
  // 48x72 was too small for the operator to tell editions apart (2026-10-07).
  posterThumb: { width: 84, height: 126, flexShrink: 0 },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 4 },
  filterChip: { paddingHorizontal: 12, paddingVertical: 8 },
  noteInput: { paddingHorizontal: 12, paddingVertical: 8, fontSize: 13, minHeight: 40, textAlignVertical: "top" },
  actionBtn: { flex: 1, paddingVertical: 10, borderRadius: 8, alignItems: "center" },
  modeChip: { paddingHorizontal: 10, paddingVertical: 6 },
  backdrop: { ...StyleSheet.absoluteFillObject, backgroundColor: "rgba(0,0,0,0.5)" },
  sheet: { position: "absolute", bottom: 0, left: 0, right: 0, padding: 20, paddingBottom: 32, maxHeight: "80%" },
  userRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth },
});
