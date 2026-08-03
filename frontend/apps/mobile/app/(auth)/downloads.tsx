import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, Text, FlatList, Pressable, Alert, StyleSheet, ActivityIndicator } from "react-native";
import { router, Stack, useFocusEffect } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useTheme } from "../../src/providers/ThemeProvider";
import {
  getDownloads,
  deleteDownload,
  cancelDownload,
  getTotalDownloadSize,
  subscribeToProgress,
  getInFlightProgress,
  getInFlightBytes,
  getInFlightDownloads,
  type DownloadedMedia,
  type ProgressBytes,
} from "../../src/lib/downloads";
import { downloadStageLabel } from "../../src/lib/downloadStageLabel";
import { canPlayOffline, type OfflineDenialReason, type OfflineProfileInfo } from "../../src/lib/offlineAccess";
import { getAudioPassState, subscribeToAudioPass, type AudioPassState } from "../../src/lib/audioCompatPass";

// A download still running is invisible in getDownloads() - an item only
// enters that persisted index once it completes - so this screen merges in
// the in-flight ones as placeholder rows (no file on disk yet, no delete
// action) until they finish and get picked up by the next refresh().
interface DownloadListItem extends DownloadedMedia {
  inProgress?: boolean;
}

// Translated label for a denial reason - deletion always stays available
// regardless of the reason (point 5 of the offline-access fix: the user must
// be able to reclaim the disk space of a download they can no longer play).
function denialLabel(t: ReturnType<typeof useTranslation>["t"], reason: OfflineDenialReason): string {
  switch (reason) {
    case "expired":
      return t("downloads.badge.expired", "Expired - re-download to watch offline again");
    case "age-gated":
      return t("downloads.badge.ageGated", "Not available for this profile");
    case "more-restricted-profile":
      return t("downloads.badge.moreRestricted", "Downloaded by another profile");
    case "unknown-profile":
      return t("downloads.badge.unavailable", "Unavailable offline");
  }
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

// Status line of a running download: percent when the total is known, plus
// the moving byte counter and transfer rate as soon as bytes flow. Falls back
// to a plain "Downloading…" only before the first progress event.
function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

interface DownloadRowProps {
  item: DownloadListItem;
  onDelete: (mediaItemId: string) => void;
  onCancel: (mediaItemId: string) => void;
  deleting: boolean;
  canceling: boolean;
  activeProfile: OfflineProfileInfo;
}

function DownloadRow({ item, onDelete, onCancel, deleting, canceling, activeProfile }: DownloadRowProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  // Live progress for a download that's still running: either a placeholder
  // row for an in-flight download (see the merge in DownloadsScreen below) or
  // one resumed from rehydrateBackgroundDownloads right after the app was
  // killed, in the brief window before it settles into the persisted index.
  const [progress, setProgress] = useState<number | null>(getInFlightProgress(item.mediaItemId));
  // Raw byte counters and a smoothed transfer rate: the integer percent of a
  // multi-GB file sits at "0%" for minutes even while bytes are flowing, and
  // that reads as "stuck". The moving MB counter and rate are what tell a
  // slow-but-healthy download apart from a dead one.
  const [bytes, setBytes] = useState<ProgressBytes | null>(getInFlightBytes(item.mediaItemId));
  const [rate, setRate] = useState(0);
  const rateRef = useRef<{ at: number; downloaded: number; rate: number } | null>(null);

  useEffect(() => {
    return subscribeToProgress(item.mediaItemId, (pct, b) => {
      setProgress(pct);
      if (!b) return;
      setBytes(b);
      const now = Date.now();
      const prev = rateRef.current;
      if (prev && now > prev.at && b.downloaded >= prev.downloaded) {
        const instant = ((b.downloaded - prev.downloaded) / (now - prev.at)) * 1000;
        // Light exponential smoothing so the figure doesn't jitter with the
        // ~1s native batching.
        const smoothed = prev.rate > 0 ? prev.rate * 0.7 + instant * 0.3 : instant;
        rateRef.current = { at: now, downloaded: b.downloaded, rate: smoothed };
        setRate(smoothed);
      } else {
        rateRef.current = { at: now, downloaded: b.downloaded, rate: prev?.rate ?? 0 };
      }
    });
  }, [item.mediaItemId]);

  // Post-download audio pass, if one is rewriting this file right now. The
  // subscription is a plain version bump (state lives in audioCompatPass.ts);
  // reading back on each bump keeps this row in sync without polling.
  const [audioPass, setAudioPass] = useState<AudioPassState | null>(getAudioPassState(item.mediaItemId));
  useEffect(() => {
    return subscribeToAudioPass(() => setAudioPass(getAudioPassState(item.mediaItemId)));
  }, [item.mediaItemId]);

  // An in-flight placeholder has no downloadedAt/rating yet to judge - only
  // gate completed entries. Deletion (below) always stays available so the
  // user can reclaim disk space for something they can no longer play.
  const decision = item.inProgress ? { allowed: true as const } : canPlayOffline(item, activeProfile, Date.now());
  const blocked = !decision.allowed;

  return (
    <Pressable
      onPress={() => { if (!item.inProgress && !blocked) router.push(`/media/${item.mediaItemId}`); }}
      style={{
        flexDirection: "row",
        alignItems: "center",
        padding: theme.spacing.md,
        gap: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
        backgroundColor: theme.colors.surface,
        opacity: blocked ? 0.5 : 1,
      }}
      accessible
      accessibilityLabel={item.title}
      accessibilityRole="button"
    >
      <View style={{
        width: 40,
        height: 40,
        borderRadius: theme.radii.sm,
        backgroundColor: theme.colors.surfaceRaised,
        alignItems: "center",
        justifyContent: "center",
      }}>
        <Ionicons name={item.inProgress ? "cloud-download-outline" : "download"} size={18} color={theme.colors.accent} />
      </View>

      <View style={{ flex: 1, gap: 2 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "500" }} numberOfLines={1}>
          {item.title}
        </Text>
        {item.inProgress ? (
          <Text style={{ color: theme.colors.accent, fontSize: 12 }}>
            {downloadStageLabel(t, item.mediaItemId, progress, bytes, rate, { formatSize })}
          </Text>
        ) : (
          <>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
              {formatSize(item.fileSize)} · {formatDate(item.downloadedAt)}
            </Text>
            {progress != null && Number.isFinite(progress) && (
              <Text style={{ color: theme.colors.accent, fontSize: 11 }}>
                {t("downloads.inProgress", "{{percent}}%", { percent: Math.round(progress * 100) })}
              </Text>
            )}
            {audioPass && (
              <Text style={{ color: theme.colors.accent, fontSize: 11 }}>
                {t("downloads.optimizingAudio", "Optimizing audio… {{percent}}%", {
                  percent: Math.round(audioPass.permille / 10),
                })}
              </Text>
            )}
            {blocked && decision.reason && (
              <Text style={{ color: theme.colors.warning, fontSize: 11, fontWeight: "600" }}>
                {denialLabel(t, decision.reason)}
              </Text>
            )}
          </>
        )}
      </View>

      {item.inProgress ? (
        <View style={{ flexDirection: "row", alignItems: "center", gap: theme.spacing.sm }}>
          <ActivityIndicator size="small" color={theme.colors.accent} />
          <Pressable
            onPress={() => onCancel(item.mediaItemId)}
            disabled={canceling}
            hitSlop={8}
            accessible
            accessibilityLabel={t("downloads.cancel", "Cancel download")}
            accessibilityRole="button"
            style={{ opacity: canceling ? 0.5 : 1 }}
          >
            <Ionicons name="close-circle-outline" size={20} color={theme.colors.textMuted} />
          </Pressable>
        </View>
      ) : (
        <Pressable
          onPress={() => onDelete(item.mediaItemId)}
          disabled={deleting}
          hitSlop={8}
          accessible
          accessibilityLabel={t("downloads.delete", "Delete download")}
          accessibilityRole="button"
          style={{ opacity: deleting ? 0.5 : 1 }}
        >
          <Ionicons name="trash-outline" size={20} color={theme.colors.textMuted} />
        </Pressable>
      )}
    </Pressable>
  );
}

function downloadKeyExtractor(item: DownloadListItem) {
  return item.mediaItemId;
}

export default function DownloadsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [items, setItems] = useState<DownloadListItem[]>([]);
  const [totalSize, setTotalSize] = useState(0);
  const [loading, setLoading] = useState(true);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [cancelingId, setCancelingId] = useState<string | null>(null);
  const [hasInFlight, setHasInFlight] = useState(false);

  // Same shape used by canPlayOffline (see offlineAccess.ts) - built here from
  // the active profile so each row can be marked as expired/not-playable for
  // this profile without changing the persisted download itself.
  const profileId = useAuthStore((s) => s.profileId);
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } } as never);
  const activeProfile: OfflineProfileInfo = useMemo(() => {
    const profile = profilesResp?.profiles?.find((p) => p.id === profileId);
    return { id: profileId, isKids: profile?.isKids, maxAge: profile?.maxAge, region: profile?.region };
  }, [profileId, profilesResp]);

  const refresh = useCallback(async () => {
    const [downloads, total] = await Promise.all([getDownloads(), getTotalDownloadSize()]);
    const completedIds = new Set(downloads.map((d) => d.mediaItemId));
    // A download not yet in the completed index (still running) is otherwise
    // invisible on this screen until it finishes - list it as a placeholder
    // row instead, ahead of the completed ones.
    const inFlightOnly = getInFlightDownloads()
      .filter((d) => !completedIds.has(d.mediaItemId))
      .map((d): DownloadListItem => ({
        mediaItemId: d.mediaItemId,
        title: d.title,
        filePath: "",
        fileSize: 0,
        downloadedAt: new Date().toISOString(),
        inProgress: true,
      }));
    setItems([...inFlightOnly, ...downloads]);
    setTotalSize(total);
    setHasInFlight(inFlightOnly.length > 0);
    setLoading(false);
  }, []);

  // Re-list on every focus - a download started elsewhere (media detail
  // screen) or completed in the background should show up without a
  // manual pull-to-refresh.
  useFocusEffect(
    useCallback(() => {
      void refresh();
    }, [refresh]),
  );

  // While an in-flight placeholder is showing, poll so it flips over to the
  // completed row (with its real size/date) shortly after the download
  // finishes, without requiring the user to leave and refocus this screen.
  useEffect(() => {
    if (!hasInFlight) return;
    const interval = setInterval(() => { void refresh(); }, 1500);
    return () => clearInterval(interval);
  }, [hasInFlight, refresh]);

  const handleDelete = useCallback((mediaItemId: string) => {
    const item = items.find((i) => i.mediaItemId === mediaItemId);
    Alert.alert(
      t("downloads.deleteTitle", "Delete download"),
      t("downloads.deleteConfirm", "Remove \"{{title}}\" from this device?", { title: item?.title ?? "" }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: async () => {
            setDeletingId(mediaItemId);
            try {
              await deleteDownload(mediaItemId);
              await refresh();
            } finally {
              setDeletingId(null);
            }
          },
        },
      ],
    );
  }, [items, refresh, t]);

  const handleCancel = useCallback((mediaItemId: string) => {
    const item = items.find((i) => i.mediaItemId === mediaItemId);
    Alert.alert(
      t("downloads.cancelTitle", "Cancel download"),
      t("downloads.cancelConfirm", "Stop downloading \"{{title}}\"?", { title: item?.title ?? "" }),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("downloads.cancelAction", "Stop"),
          style: "destructive",
          onPress: async () => {
            setCancelingId(mediaItemId);
            try {
              await cancelDownload(mediaItemId);
              await refresh();
            } finally {
              setCancelingId(null);
            }
          },
        },
      ],
    );
  }, [items, refresh, t]);

  const renderItem = useCallback(
    ({ item }: { item: DownloadListItem }) => (
      <DownloadRow
        item={item}
        onDelete={handleDelete}
        onCancel={handleCancel}
        deleting={deletingId === item.mediaItemId}
        canceling={cancelingId === item.mediaItemId}
        activeProfile={activeProfile}
      />
    ),
    [handleDelete, handleCancel, deletingId, cancelingId, activeProfile],
  );

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("downloads.title", "Downloads"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {items.length > 0 && (
        <Text style={{
          color: theme.colors.textMuted,
          fontSize: 12,
          paddingHorizontal: theme.spacing.md,
          paddingBottom: theme.spacing.sm,
        }}>
          {t("downloads.totalSize", "{{size}} used on this device", { size: formatSize(totalSize) })}
        </Text>
      )}

      {!loading && items.length === 0 ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: theme.spacing.xl }}>
          <Ionicons name="download-outline" size={48} color={theme.colors.textMuted} />
          <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: theme.spacing.md, textAlign: "center" }}>
            {t("downloads.empty", "No downloads yet")}
          </Text>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={downloadKeyExtractor}
          renderItem={renderItem}
          contentContainerStyle={{ paddingBottom: theme.spacing.xl }}
        />
      )}
    </View>
  );
}
