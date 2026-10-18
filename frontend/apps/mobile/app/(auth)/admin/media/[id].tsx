import { useState, useEffect } from "react";
import {
  ScrollView,
  View,
  Text,
  TextInput,
  Pressable,
  ActivityIndicator,
  StyleSheet,
  Alert,
} from "react-native";
import { Stack } from "expo-router";
import { useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useLibraryServiceGetMediaItem, itemLabel, eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { HeaderBack } from "../../../../src/components/layout/HeaderBack";
import { useServerUrl } from "../../../../src/hooks/useServerUrl";
import { SegmentPlayer } from "../../../../src/components/admin/SegmentPlayer";
import {
  useAdminMediaSegments,
  useSetSegment,
  useDeleteSegment,
  usePropagateSegment,
  useTriggerAutoDetect,
} from "../../../../src/hooks/useAdminSegments";
import type { MediaSegment } from "../../../../src/hooks/useAdminSegments";

const SEGMENT_TYPES = ["intro", "recap", "end_credits", "studio_logo"] as const;
type SegmentType = (typeof SEGMENT_TYPES)[number];

interface LocalSegment extends MediaSegment {
  isDirty: boolean;
}

function msToMmSs(ms: number): string {
  const totalSec = Math.floor(ms / 1000);
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m.toString().padStart(2, "0")}:${s.toString().padStart(2, "0")}`;
}

function mmSsToMs(value: string): number | null {
  const parts = value.split(":");
  if (parts.length !== 2) return null;
  const m = parseInt(parts[0], 10);
  const s = parseInt(parts[1], 10);
  if (isNaN(m) || isNaN(s)) return null;
  return (m * 60 + s) * 1000;
}

function isManualSource(source: string): boolean {
  return source === "manual" || source === "manual_propagated";
}

export default function AdminMediaSegmentsScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const { t } = useTranslation();

  const { data: mediaItem, isLoading: itemLoading } = useLibraryServiceGetMediaItem(id ?? "");
  const { data: serverSegments, isLoading: segLoading } = useAdminMediaSegments(id);

  const setSegment = useSetSegment(id ?? "");
  const deleteSegment = useDeleteSegment(id ?? "");
  const propagate = usePropagateSegment(id ?? "");
  const triggerDetect = useTriggerAutoDetect(id ?? "");

  const [localSegments, setLocalSegments] = useState<LocalSegment[]>([]);
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const serverUrl = useServerUrl();

  const isEpisode = mediaItem?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const isLoading = itemLoading || segLoading;

  // Mint a stream token so the native player can fetch the raw file (mirror web).
  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetch<{ token: string }>({ url: `/api/v1/media/${id}/stream-token`, method: "POST" });
        if (!cancelled) setStreamToken(res.token);
      } catch {
        if (!cancelled) setStreamToken(null);
      }
    })();
    return () => { cancelled = true; };
  }, [id]);

  const streamUrl = streamToken ? `${serverUrl}/api/v1/media/${id}/stream?token=${encodeURIComponent(streamToken)}` : null;
  const mediaDurationMs = Number(mediaItem?.durationMs ?? 0);

  // Mark start/end from the player's playhead onto the selected segment.
  function handleMark(edge: "start" | "end", ms: number) {
    if (!selectedType) return;
    setLocalSegments((prev) => prev.map((sg) => {
      if (sg.type !== selectedType) return sg;
      const startMs = edge === "start" ? ms : sg.startMs;
      const endMs = edge === "end" ? ms : sg.endMs;
      return { ...sg, startMs, endMs, isDirty: true };
    }));
  }

  // Sync server segments → local state (don't overwrite dirty entries)
  useEffect(() => {
    if (!serverSegments) return;
    setLocalSegments((prev) => {
      const dirtyTypes = new Set(prev.filter((s) => s.isDirty).map((s) => s.type));
      const updated: LocalSegment[] = serverSegments.map((ss) => {
        if (dirtyTypes.has(ss.type)) {
          return prev.find((p) => p.type === ss.type)!;
        }
        return { ...ss, isDirty: false };
      });
      for (const p of prev) {
        if (p.isDirty && !updated.find((u) => u.type === p.type)) {
          updated.push(p);
        }
      }
      return updated;
    });
  }, [serverSegments]);

  // Keep a valid segment selected so the mark buttons are usable.
  useEffect(() => {
    if (localSegments.length === 0) { if (selectedType) setSelectedType(null); return; }
    if (!selectedType || !localSegments.find((sg) => sg.type === selectedType)) {
      setSelectedType(localSegments[0].type);
    }
  }, [localSegments, selectedType]);

  async function handleAutoDetect() {
    try {
      await triggerDetect.mutateAsync();
      toast.success(t("admin.segments.redetectSuccess"));
    } catch {
      toast.error(t("admin.segments.redetectError"));
    }
  }

  async function handleSave(seg: LocalSegment) {
    const startMs = mmSsToMs(msToMmSs(seg.startMs));
    const endMs = mmSsToMs(msToMmSs(seg.endMs));
    if (startMs === null || endMs === null || startMs >= endMs) return;
    try {
      await setSegment.mutateAsync({ type: seg.type, startMs: seg.startMs, endMs: seg.endMs });
      setLocalSegments((prev) =>
        prev.map((s) => (s.type === seg.type ? { ...s, isDirty: false } : s)),
      );
      toast.success(t("admin.segments.saved"));
    } catch {
      toast.error(t("admin.segments.saveError"));
    }
  }

  function handleDelete(seg: LocalSegment) {
    Alert.alert(
      t("common.delete", "Delete"),
      t("admin.segments.delete", "Delete segment") + ` "${seg.type.replace(/_/g, " ")}"?`,
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        {
          text: t("common.delete", "Delete"),
          style: "destructive",
          onPress: async () => {
            try {
              await deleteSegment.mutateAsync(seg.type);
              setLocalSegments((prev) => prev.filter((s) => s.type !== seg.type));
              toast.success(t("admin.segments.deleted"));
            } catch {
              toast.error(t("admin.segments.deleteError"));
            }
          },
        },
      ],
    );
  }

  async function handlePropagate(type: string) {
    try {
      const res = await propagate.mutateAsync(type);
      toast.success(
        t("admin.segments.propagateSuccess", `Propagated to ${res.count} episode(s)`, {
          count: res.count,
        }),
      );
    } catch {
      toast.error(t("common.error", "An error occurred"));
    }
  }

  function handleAddSegment(type: SegmentType) {
    if (localSegments.find((s) => s.type === type)) return;
    setLocalSegments((prev) => [
      ...prev,
      { type, startMs: 0, endMs: 5000, confidence: 1, source: "manual", isDirty: true },
    ]);
  }

  const availableToAdd = SEGMENT_TYPES.filter(
    (type) => !localSegments.find((s) => s.type === type),
  );

  const s = makeStyles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={s.content}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: (mediaItem ? itemLabel(mediaItem, t) : "") || t("admin.segments.title", "Segment Admin"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerLeft: () => <HeaderBack />,
        }}
      />

      {isLoading ? (
        <View style={s.centered}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <>
          {/* Auto-detect button */}
          <Pressable
            style={[s.detectBtn, triggerDetect.isPending && s.btnDisabled]}
            onPress={handleAutoDetect}
            disabled={triggerDetect.isPending}
          >
            {triggerDetect.isPending ? (
              <ActivityIndicator color={theme.colors.onAccent} size="small" />
            ) : (
              <Text style={s.detectBtnText}>
                {t("admin.segments.redetect", "Auto-detect (re-run pipeline)")}
              </Text>
            )}
          </Pressable>

          {/* Section A: native player + mark start/end + visual timeline. */}
          {streamUrl ? (
            <SegmentPlayer
              streamUrl={streamUrl}
              durationMsFallback={mediaDurationMs}
              segments={localSegments.map((sg) => ({ type: sg.type, startMs: sg.startMs, endMs: sg.endMs }))}
              selectedType={selectedType}
              onSelectType={setSelectedType}
              onMark={handleMark}
            />
          ) : null}

          {/* Segment rows */}
          {localSegments.length === 0 ? (
            <Text style={s.emptyText}>
              {t(
                "admin.segments.empty",
                "No segments detected. Add one below or trigger auto-detect.",
              )}
            </Text>
          ) : (
            localSegments.map((seg) => (
              <SegmentRow
                key={seg.type}
                seg={seg}
                isEpisode={isEpisode}
                isSavePending={setSegment.isPending}
                isDeletePending={deleteSegment.isPending}
                isPropagatePending={propagate.isPending}
                theme={theme}
                t={t}
                onSave={handleSave}
                onDelete={handleDelete}
                onPropagate={handlePropagate}
                onLocalChange={(updated) =>
                  setLocalSegments((prev) =>
                    prev.map((s) => (s.type === updated.type ? updated : s)),
                  )
                }
              />
            ))
          )}

          {/* Add missing segment types */}
          {availableToAdd.length > 0 && (
            <View style={s.addSection}>
              <Text style={s.addLabel}>{t("admin.segments.add", "Add segment:")}</Text>
              <View style={s.addRow}>
                {availableToAdd.map((type) => (
                  <Pressable
                    key={type}
                    style={s.addChip}
                    onPress={() => handleAddSegment(type)}
                  >
                    <Text style={s.addChipText}>{type.replace(/_/g, " ")}</Text>
                  </Pressable>
                ))}
              </View>
            </View>
          )}

          {/* Locked hint */}
          <Text style={s.lockedHint}>
            {t(
              "admin.segments.lockedHint",
              "Segments marked with a lock were set manually and will not be overwritten by auto-detect.",
            )}
          </Text>
        </>
      )}
    </ScrollView>
  );
}

// Segment row sub-component - owns the local mm:ss state
function SegmentRow({
  seg,
  isEpisode,
  isSavePending,
  isDeletePending,
  isPropagatePending,
  theme,
  t,
  onSave,
  onDelete,
  onPropagate,
  onLocalChange,
}: {
  seg: LocalSegment;
  isEpisode: boolean;
  isSavePending: boolean;
  isDeletePending: boolean;
  isPropagatePending: boolean;
  theme: ReturnType<typeof useTheme>;
  t: ReturnType<typeof useTranslation>["t"];
  onSave: (seg: LocalSegment) => void;
  onDelete: (seg: LocalSegment) => void;
  onPropagate: (type: string) => void;
  onLocalChange: (seg: LocalSegment) => void;
}) {
  const [startVal, setStartVal] = useState(msToMmSs(seg.startMs));
  const [endVal, setEndVal] = useState(msToMmSs(seg.endMs));

  // Re-sync when server pushes fresh data (isDirty reset to false)
  useEffect(() => {
    if (!seg.isDirty) {
      setStartVal(msToMmSs(seg.startMs));
      setEndVal(msToMmSs(seg.endMs));
    }
  }, [seg.isDirty, seg.startMs, seg.endMs]);

  const parsedStart = mmSsToMs(startVal);
  const parsedEnd = mmSsToMs(endVal);
  const isValid =
    parsedStart !== null && parsedEnd !== null && parsedStart < parsedEnd;

  function commitStart(value: string) {
    const ms = mmSsToMs(value);
    if (ms !== null) {
      onLocalChange({ ...seg, startMs: ms, isDirty: true });
    }
  }

  function commitEnd(value: string) {
    const ms = mmSsToMs(value);
    if (ms !== null) {
      onLocalChange({ ...seg, endMs: ms, isDirty: true });
    }
  }

  const isManual = isManualSource(seg.source);
  const s = makeStyles(theme);

  return (
    <View style={s.segCard}>
      <View style={s.segHeader}>
        <Text style={s.segType}>{seg.type.replace(/_/g, " ")}</Text>
        {isManual && (
          <View style={s.lockBadge}>
            <Text style={s.lockText}>{t("admin.segments.locked", "locked")}</Text>
          </View>
        )}
        <Text style={s.segSource}>{seg.source}</Text>
      </View>

      <View style={s.inputRow}>
        <View style={s.inputGroup}>
          <Text style={s.inputLabel}>{t("admin.segments.start", "Start mm:ss")}</Text>
          <TextInput
            style={[s.input, !isValid && parsedStart === null && s.inputError]}
            accessibilityLabel={t("admin.segments.start", "Start mm:ss")}
            value={startVal}
            onChangeText={(v) => setStartVal(v)}
            onBlur={() => commitStart(startVal)}
            keyboardType="numbers-and-punctuation"
            placeholder="00:00"
            placeholderTextColor={theme.colors.textMuted}
            maxLength={5}
          />
        </View>
        <View style={s.inputGroup}>
          <Text style={s.inputLabel}>{t("admin.segments.end", "End mm:ss")}</Text>
          <TextInput
            style={[s.input, !isValid && parsedEnd === null && s.inputError]}
            accessibilityLabel={t("admin.segments.end", "End mm:ss")}
            value={endVal}
            onChangeText={(v) => setEndVal(v)}
            onBlur={() => commitEnd(endVal)}
            keyboardType="numbers-and-punctuation"
            placeholder="00:30"
            placeholderTextColor={theme.colors.textMuted}
            maxLength={5}
          />
        </View>
      </View>

      <View style={s.actionRow}>
        <Pressable
          style={[s.saveBtn, (!isValid || isSavePending) && s.btnDisabled]}
          onPress={() => isValid && onSave({ ...seg, startMs: parsedStart!, endMs: parsedEnd! })}
          disabled={!isValid || isSavePending}
        >
          <Text style={s.saveBtnText}>{t("admin.segments.save", "Save")}</Text>
        </Pressable>

        <Pressable
          style={[s.deleteBtn, isDeletePending && s.btnDisabled]}
          onPress={() => onDelete(seg)}
          disabled={isDeletePending}
        >
          <Text style={s.deleteBtnText}>{t("admin.segments.delete", "Delete")}</Text>
        </Pressable>

        {isEpisode && isManual && (
          <Pressable
            style={[s.propagateBtn, isPropagatePending && s.btnDisabled]}
            onPress={() => onPropagate(seg.type)}
            disabled={isPropagatePending}
          >
            <Text style={s.propagateBtnText}>
              {t("admin.segments.propagate", "Propagate to season")}
            </Text>
          </Pressable>
        )}
      </View>
    </View>
  );
}

// eslint-disable-next-line @typescript-eslint/explicit-function-return-type
function makeStyles(theme: ReturnType<typeof useTheme>) {
  return StyleSheet.create({
    container: { flex: 1, backgroundColor: theme.colors.bg },
    content: { padding: 16, paddingBottom: 40 },
    centered: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
    detectBtn: {
      backgroundColor: theme.colors.accent,
      borderRadius: theme.radii.md,
      paddingVertical: 14,
      alignItems: "center",
      marginBottom: 20,
    },
    detectBtnText: { color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 },
    emptyText: { color: theme.colors.textMuted, textAlign: "center", marginTop: 32, fontSize: 14 },
    segCard: {
      backgroundColor: theme.colors.surface,
      borderRadius: theme.radii.md,
      padding: 14,
      marginBottom: 14,
    },
    segHeader: { flexDirection: "row", alignItems: "center", marginBottom: 10, gap: 8 },
    segType: { color: theme.colors.text, fontWeight: "700", fontSize: 15, flex: 1, textTransform: "capitalize" },
    lockBadge: {
      backgroundColor: theme.colors.warning,
      borderRadius: theme.radii.sm,
      paddingHorizontal: 6,
      paddingVertical: 2,
    },
    lockText: { color: theme.colors.onWarning, fontSize: 10, fontWeight: "700" },
    segSource: { color: theme.colors.textMuted, fontSize: 11 },
    inputRow: { flexDirection: "row", gap: 12, marginBottom: 12 },
    inputGroup: { flex: 1 },
    inputLabel: { color: theme.colors.textMuted, fontSize: 12, marginBottom: 4 },
    input: {
      backgroundColor: theme.colors.surfaceRaised,
      borderColor: theme.colors.border,
      borderWidth: 1,
      borderRadius: theme.radii.sm,
      color: theme.colors.text,
      paddingHorizontal: 10,
      paddingVertical: 8,
      fontSize: 14,
      fontFamily: "monospace",
    },
    inputError: { borderColor: theme.colors.error },
    actionRow: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
    saveBtn: {
      backgroundColor: theme.colors.accent,
      borderRadius: theme.radii.sm,
      paddingHorizontal: 16,
      paddingVertical: 8,
    },
    saveBtnText: { color: theme.colors.onAccent, fontWeight: "600", fontSize: 13 },
    deleteBtn: {
      backgroundColor: theme.colors.danger,
      borderRadius: theme.radii.sm,
      paddingHorizontal: 16,
      paddingVertical: 8,
    },
    deleteBtnText: { color: theme.colors.onDanger, fontWeight: "600", fontSize: 13 },
    propagateBtn: {
      backgroundColor: theme.colors.surfaceRaised,
      borderRadius: theme.radii.sm,
      paddingHorizontal: 16,
      paddingVertical: 8,
      borderWidth: 1,
      borderColor: theme.colors.border,
    },
    propagateBtnText: { color: theme.colors.text, fontWeight: "600", fontSize: 13 },
    btnDisabled: { opacity: 0.5 },
    addSection: { marginTop: 8, marginBottom: 16 },
    addLabel: { color: theme.colors.textMuted, fontSize: 12, marginBottom: 8 },
    addRow: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
    addChip: {
      backgroundColor: theme.colors.surfaceRaised,
      borderRadius: theme.radii.sm,
      paddingHorizontal: 12,
      paddingVertical: 6,
      borderWidth: 1,
      borderColor: theme.colors.border,
    },
    addChipText: { color: theme.colors.text, fontSize: 13, textTransform: "capitalize" },
    lockedHint: {
      color: theme.colors.textMuted,
      fontSize: 11,
      marginTop: 8,
      lineHeight: 16,
    },
  });
}
