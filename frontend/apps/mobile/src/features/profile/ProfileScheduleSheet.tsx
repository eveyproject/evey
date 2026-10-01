import { useEffect, useState } from "react";
import { View, Text, Pressable, Modal, ScrollView, Switch, TextInput, StyleSheet, ActivityIndicator } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiErrorMessage,
  getProfileSchedule,
  hhmmToMinutes,
  minutesToHHMM,
  profileScheduleKey,
  rangeCrossesMidnight,
  scheduleProblems,
  updateProfileSchedule,
  type ProfileScheduleState,
  type ScheduleMode,
  type ScheduleRange,
} from "@evey/api";
import { blocksFor, formatOpening, formatTime, nextRange, scheduleFromBlocks, weekdayName, type ScheduleBlock } from "@evey/client";
import { toast, useDemoStore } from "@evey/store";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { useTheme } from "../../providers/ThemeProvider";

interface Props {
  profileId: string;
  profileName: string;
  onClose: () => void;
}

// A kids profile's opening hours, same rules and views as the web: weekdays
// and weekend blocks, or one block per day. The server enforces them.
export function ProfileScheduleSheet({ profileId, profileName, onClose }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data } = useQuery({
    queryKey: profileScheduleKey(profileId),
    queryFn: ({ signal }) => getProfileSchedule(profileId, signal),
  });
  return (
    <Modal visible animationType="slide" onRequestClose={onClose} presentationStyle="pageSheet">
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <View style={[styles.header, { borderBottomColor: theme.colors.border }]}>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
            {t("auth.profile.schedule.title", "Opening hours")} - {profileName}
          </Text>
          <Pressable onPress={onClose} accessibilityRole="button" accessibilityLabel={t("common.close", "Close")} style={styles.iconBtn}>
            <Ionicons name="close" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
          </Pressable>
        </View>
        {!data ? (
          <ActivityIndicator style={{ marginTop: 40 }} color={theme.colors.accent} />
        ) : (
          <Editor profileId={profileId} state={data} onDone={onClose} />
        )}
      </View>
    </Modal>
  );
}

function Editor({ profileId, state, onDone }: { profileId: string; state: ProfileScheduleState; onDone: () => void }) {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const demo = useDemoStore((s) => s.demoMode);
  const initial = state.schedule ?? { ...state.proposal, enabled: false };
  const [enabled, setEnabled] = useState(initial.enabled);
  const [mode, setMode] = useState<ScheduleMode>(initial.mode);
  const [blocks, setBlocks] = useState<ScheduleBlock[]>(() => blocksFor(initial, initial.mode) ?? blocksFor(initial, "daily")!);
  const [modeError, setModeError] = useState(false);
  const [saving, setSaving] = useState(false);
  useEffect(() => setModeError(false), [mode]);

  const current = scheduleFromBlocks(enabled, mode, blocks);
  const problems = scheduleProblems({ enabled, mode, rules: blocks.map((b) => ({ days: b.days, ranges: b.ranges })) });
  const editable = enabled && !demo;

  function switchMode(next: ScheduleMode) {
    if (next === mode) return;
    const nextBlocks = blocksFor(current, next);
    if (!nextBlocks) {
      setModeError(true);
      return;
    }
    setMode(next);
    setBlocks(nextBlocks);
  }

  async function save() {
    setSaving(true);
    try {
      const saved = await updateProfileSchedule(profileId, current);
      qc.setQueryData(profileScheduleKey(profileId), saved);
      toast.success(t("auth.profile.schedule.saved", "Opening hours saved"));
      onDone();
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("auth.profile.schedule.saveFailed", "Could not save the opening hours")));
    } finally {
      setSaving(false);
    }
  }

  const label = (b: ScheduleBlock) =>
    b.key === "weekdays"
      ? t("auth.profile.schedule.weekdays", "Monday to Friday")
      : b.key === "weekend"
        ? t("auth.profile.schedule.weekend", "Saturday and Sunday")
        : weekdayName(b.days[0], i18n.language);

  const a = state.access;
  let status: string | null = null;
  if (state.schedule?.enabled) {
    if (a.overridden) status = t("auth.profile.schedule.statusOverridden", "Opened by a parent until the next opening");
    else if (a.open && a.closesAt) status = t("auth.profile.schedule.statusOpenUntil", "Open now, until {{time}}", { time: formatTime(a.closesAt, i18n.language) });
    else if (!a.open && a.nextOpen) status = t("auth.profile.schedule.statusClosedUntil", "Locked now, opens {{when}}", { when: formatOpening(a.nextOpen, i18n.language) });
    else status = t("auth.profile.schedule.statusClosed", "Locked now");
  }

  return (
    <ScrollView contentContainerStyle={{ padding: theme.spacing.md, gap: theme.spacing.md }} keyboardShouldPersistTaps="handled">
      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.sm }}>
        {t("auth.profile.schedule.description", "Choose when this kids profile can be used. Outside these hours it locks on every device, playback included.")}
      </Text>
      {status && (
        <Text accessibilityRole="text" style={{ color: a.open ? theme.colors.success : theme.colors.warning, fontWeight: "600", fontSize: FONT_SIZE.sm }}>
          {status}
        </Text>
      )}
      <View style={styles.row}>
        <Text style={{ flex: 1, color: theme.colors.text, fontSize: FONT_SIZE.sm, fontWeight: "500" }}>
          {t("auth.profile.schedule.enable", "Limit to opening hours")}
        </Text>
        <Switch
          value={enabled}
          onValueChange={setEnabled}
          disabled={demo}
          accessibilityLabel={t("auth.profile.schedule.enable", "Limit to opening hours")}
          trackColor={{ true: theme.colors.accent, false: theme.colors.border }}
        />
      </View>

      <View style={{ opacity: enabled ? 1 : 0.5, gap: theme.spacing.md }} pointerEvents={editable ? "auto" : "none"}>
        <View accessibilityRole="radiogroup" style={[styles.segment, { borderColor: theme.colors.border, borderRadius: theme.radii.md }]}>
          {(["week", "daily"] as const).map((m) => (
            <Pressable
              key={m}
              onPress={() => switchMode(m)}
              accessibilityRole="radio"
              accessibilityState={{ checked: mode === m }}
              style={[styles.segmentBtn, { borderRadius: theme.radii.sm, backgroundColor: mode === m ? theme.colors.accent : "transparent" }]}
            >
              <Text style={{ color: mode === m ? theme.colors.onAccent : theme.colors.textMuted, fontSize: FONT_SIZE.sm, fontWeight: "600" }}>
                {m === "week" ? t("auth.profile.schedule.modeWeek", "Weekdays / weekend") : t("auth.profile.schedule.modeDaily", "Day by day")}
              </Text>
            </Pressable>
          ))}
        </View>
        {modeError && (
          <Text accessibilityRole="alert" style={{ color: theme.colors.warning, fontSize: FONT_SIZE.xs }}>
            {t("auth.profile.schedule.cannotWeek", "Some days differ from the rest of their block: keep the day-by-day view, or make them alike first.")}
          </Text>
        )}
        {blocks.map((b) => (
          <BlockEditor
            key={b.key}
            label={label(b)}
            ranges={b.ranges}
            onChange={(ranges) => setBlocks((bs) => bs.map((x) => (x.key === b.key ? { ...x, ranges } : x)))}
          />
        ))}
      </View>

      {enabled && problems.includes("no_rules") && (
        <Text accessibilityRole="alert" style={{ color: theme.colors.warning, fontSize: FONT_SIZE.xs }}>
          {t("auth.profile.schedule.noHours", "No time range: the profile stays locked at all times.")}
        </Text>
      )}
      {problems.includes("empty_range") && (
        <Text accessibilityRole="alert" style={{ color: theme.colors.error, fontSize: FONT_SIZE.xs }}>
          {t("auth.profile.schedule.emptyRange", "A time range must not start and end at the same time.")}
        </Text>
      )}
      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
        {t("auth.profile.schedule.timezone", "Times follow the server's time zone ({{zone}}).", { zone: state.timezone })}
      </Text>

      <Pressable
        onPress={() => {
          setEnabled(true);
          setMode(state.proposal.mode);
          setBlocks(blocksFor(state.proposal, state.proposal.mode)!);
        }}
        disabled={demo}
        accessibilityRole="button"
        style={[styles.btn, { borderColor: theme.colors.border, borderWidth: 1, borderRadius: theme.radii.md, opacity: demo ? 0.5 : 1 }]}
      >
        <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: FONT_SIZE.sm }}>
          {t("auth.profile.schedule.useProposal", "Use the suggested hours")}
        </Text>
      </Pressable>
      <Pressable
        onPress={() => void save()}
        disabled={demo || saving || problems.includes("empty_range")}
        accessibilityRole="button"
        accessibilityLabel={t("common.save", "Save")}
        style={[styles.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: demo || problems.includes("empty_range") ? 0.5 : 1 }]}
      >
        {saving ? (
          <ActivityIndicator color={theme.colors.onAccent} />
        ) : (
          <Text style={{ color: theme.colors.onAccent, fontWeight: "700", fontSize: FONT_SIZE.sm }}>{t("common.save", "Save")}</Text>
        )}
      </Pressable>
    </ScrollView>
  );
}

function BlockEditor({ label, ranges, onChange }: { label: string; ranges: ScheduleRange[]; onChange: (r: ScheduleRange[]) => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const set = (i: number, patch: Partial<ScheduleRange>) => onChange(ranges.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return (
    <View style={[styles.block, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}>
      <View style={styles.row}>
        <Text style={{ flex: 1, color: theme.colors.text, fontWeight: "600", fontSize: FONT_SIZE.sm, textTransform: "capitalize" }}>{label}</Text>
        {ranges.length === 0 && (
          <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("auth.profile.schedule.dayClosed", "Closed")}</Text>
        )}
      </View>
      {ranges.map((r, i) => (
        <View key={i} style={[styles.row, { flexWrap: "wrap", gap: 8 }]}>
          <TimeField
            label={t("auth.profile.schedule.from", "From")}
            minutes={r.start}
            onChange={(m) => m < 1440 && set(i, { start: m })}
          />
          <TimeField
            label={t("auth.profile.schedule.to", "To")}
            minutes={r.end}
            onChange={(m) => set(i, { end: m === 0 ? 1440 : m })}
          />
          {rangeCrossesMidnight(r) && r.end !== 1440 && (
            <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{t("auth.profile.schedule.nextDay", "until the next day")}</Text>
          )}
          <Pressable
            onPress={() => onChange(ranges.filter((_, j) => j !== i))}
            accessibilityRole="button"
            accessibilityLabel={t("auth.profile.schedule.removeRange", "Remove this time range")}
            style={[styles.iconBtn, { marginLeft: "auto" }]}
          >
            <Ionicons name="trash-outline" size={ICON_SIZE.md} color={theme.colors.error} />
          </Pressable>
        </View>
      ))}
      <Pressable
        onPress={() => onChange([...ranges, nextRange(ranges)])}
        accessibilityRole="button"
        style={[styles.row, { gap: 4, paddingVertical: 4 }]}
      >
        <Ionicons name="add" size={ICON_SIZE.md} color={theme.colors.accent} />
        <Text style={{ color: theme.colors.accent, fontSize: FONT_SIZE.sm }}>{t("auth.profile.schedule.addRange", "Add a time range")}</Text>
      </Pressable>
    </View>
  );
}

// "HH:MM" typed on the number pad, applied when the field is left: no native
// picker dependency for four characters.
function TimeField({ label, minutes, onChange }: { label: string; minutes: number; onChange: (m: number) => void }) {
  const theme = useTheme();
  const [text, setText] = useState(minutesToHHMM(minutes));
  useEffect(() => setText(minutesToHHMM(minutes)), [minutes]);
  const commit = () => {
    const v = hhmmToMinutes(text);
    if (v === null) setText(minutesToHHMM(minutes));
    else onChange(v);
  };
  return (
    <View style={[styles.row, { gap: 6 }]}>
      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>{label}</Text>
      <TextInput
        value={text}
        onChangeText={(v) => {
          const digits = v.replace(/\D/g, "").slice(0, 4);
          setText(digits.length > 2 ? `${digits.slice(0, 2)}:${digits.slice(2)}` : digits);
        }}
        onBlur={commit}
        onSubmitEditing={commit}
        keyboardType="number-pad"
        maxLength={5}
        accessibilityLabel={label}
        style={[styles.time, { color: theme.colors.text, borderColor: theme.colors.border, backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  header: { flexDirection: "row", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  title: { flex: 1, fontSize: FONT_SIZE.lg, fontWeight: "700" },
  iconBtn: { padding: 8 },
  row: { flexDirection: "row", alignItems: "center" },
  segment: { flexDirection: "row", alignSelf: "flex-start", borderWidth: 1, padding: 2 },
  segmentBtn: { paddingHorizontal: 12, paddingVertical: 8 },
  block: { borderWidth: 1, padding: 12, gap: 8 },
  btn: { paddingVertical: 12, alignItems: "center", justifyContent: "center" },
  time: { borderWidth: 1, paddingHorizontal: 8, paddingVertical: 6, minWidth: 64, textAlign: "center", fontSize: FONT_SIZE.sm },
});
