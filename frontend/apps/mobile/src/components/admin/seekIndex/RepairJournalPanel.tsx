import { useMemo, useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useAdminMediaServiceListMediaRepairs, baseName } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { LoadFailed } from "../../LoadFailed";
import { repairOpLabel, repairEvidenceLine } from "@evey/client";

// RepairJournalPanel mirrors the web features/admin/seekIndex/RepairJournalPanel:
// what repairs touched across the whole library - a window, a batch kind, and
// every file a repair changed, with the evidence each action rested on. The
// panel that would have found the 2026-07-13 batch rewrite in one query.

type Window = "24h" | "7d" | "30d" | "all";
const WINDOW_MS: Record<Window, number> = { "24h": 86_400_000, "7d": 7 * 86_400_000, "30d": 30 * 86_400_000, all: 0 };
const WINDOWS: Window[] = ["24h", "7d", "30d", "all"];
const WINDOW_KEYS: Record<Window, string> = {
  "24h": "admin.repairJournal.window24h",
  "7d": "admin.repairJournal.window7d",
  "30d": "admin.repairJournal.window30d",
  all: "admin.repairJournal.windowAll",
};

type JobFilter = "" | "operator" | "reindex" | "rollback" | "purge" | "fix_container" | "scan";
const JOBS: JobFilter[] = ["", "operator", "reindex", "rollback", "purge", "fix_container", "scan"];
const JOB_KEYS: Record<JobFilter, string> = {
  "": "admin.repairJournal.jobAll",
  operator: "admin.repairJournal.jobOperator",
  reindex: "admin.seekIndex.jobReindex",
  rollback: "admin.seekIndex.jobRollback",
  purge: "admin.seekIndex.jobPurge",
  fix_container: "admin.seekIndex.jobFixContainer",
  scan: "admin.seekIndex.jobScan",
};


function outcomeColor(outcome: string | undefined, theme: ReturnType<typeof useTheme>): string {
  switch (outcome) {
    case "applied": return theme.colors.success;
    case "rolled_back": return theme.colors.warning;
    case "failed": return theme.colors.error;
    default: return theme.colors.textMuted;
  }
}


function Chip({ theme, label, active, onPress }: { theme: ReturnType<typeof useTheme>; label: string; active: boolean; onPress: () => void }) {
  return (
    <Pressable
      onPress={onPress}
      style={{ paddingHorizontal: 10, paddingVertical: 5, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: active ? theme.colors.accent : theme.colors.border, backgroundColor: active ? theme.colors.accent : "transparent" }}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 12, fontWeight: "600" }}>{label}</Text>
    </Pressable>
  );
}

export function RepairJournalPanel() {
  const theme = useTheme();
  const { t } = useTranslation();
  const s = styles(theme);
  const [open, setOpen] = useState(false);
  const [win, setWin] = useState<Window>("7d");
  const [job, setJob] = useState<JobFilter>("");

  const sinceMs = useMemo(() => (WINDOW_MS[win] > 0 ? String(Date.now() - WINDOW_MS[win]) : undefined), [win]);
  const journal = useAdminMediaServiceListMediaRepairs(
    { ...(sinceMs ? { sinceMs } : {}), ...(job && job !== "operator" ? { job } : {}), limit: 200 },
    { query: { enabled: open } },
  );
  const entries = (journal.data?.entries ?? []).filter((e) => job !== "operator" || !e.job);

  return (
    <View style={[s.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: 8 }}>
        <View style={{ flex: 1 }}>
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("admin.repairJournal.title", "Repair journal")}</Text>
          <Text style={s.hint}>{t("admin.repairJournal.intro", "Every file a repair changed, library-wide.")}</Text>
        </View>
        <Pressable onPress={() => setOpen((v) => !v)}>
          <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "600" }}>
            {open ? t("admin.repairJournal.hideBtn", "Hide") : t("admin.repairJournal.showBtn", "Show")}
          </Text>
        </Pressable>
      </View>

      {open && (
        <View style={{ marginTop: 10, gap: 8 }}>
          <Text style={s.filterLabel}>{t("admin.repairJournal.windowLabel", "Window")}</Text>
          <View style={s.chipRow}>
            {WINDOWS.map((w) => <Chip key={w} theme={theme} label={t(WINDOW_KEYS[w])} active={win === w} onPress={() => setWin(w)} />)}
          </View>
          <Text style={s.filterLabel}>{t("admin.repairJournal.jobLabel", "Job")}</Text>
          <View style={s.chipRow}>
            {JOBS.map((j) => <Chip key={j} theme={theme} label={t(JOB_KEYS[j])} active={job === j} onPress={() => setJob(j)} />)}
          </View>

          {journal.isLoading ? (
            <Text style={s.hint}>{t("common.loading")}</Text>
          ) : journal.isError ? (
            <LoadFailed inline onRetry={() => void journal.refetch()} />
          ) : entries.length === 0 ? (
            <Text style={s.hint}>{t("admin.repairJournal.empty", "No repairs in this window.")}</Text>
          ) : (
            <>
              <Text style={s.hint}>{t("admin.repairJournal.count", { count: entries.length })}</Text>
              {entries.map((e, i) => (
                <View key={i} style={[s.entry, { borderTopColor: theme.colors.border }]}>
                  <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6 }}>
                    <Text style={{ color: theme.colors.text, fontSize: 12 }}>{new Date(Number(e.atMs ?? 0)).toLocaleString()}</Text>
                    <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }} numberOfLines={1}>{baseName(e.path ?? "")}</Text>
                  </View>
                  <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6, marginTop: 2 }}>
                    <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "600" }}>{repairOpLabel(e.op, t)}</Text>
                    <Text style={{ color: outcomeColor(e.outcome, theme), fontSize: 11, fontWeight: "600" }}>{t(`admin.repairHistory.outcome.${e.outcome}`)}</Text>
                    {e.job ? <Text style={s.hint}>{t(JOB_KEYS[(e.job as JobFilter) in JOB_KEYS ? (e.job as JobFilter) : ""])}</Text> : null}
                  </View>
                  <Text style={s.hint}>{repairEvidenceLine(e, t)}</Text>
                  {e.error ? <Text style={{ color: theme.colors.error, fontSize: 11 }}>{e.error}</Text> : null}
                </View>
              ))}
            </>
          )}
        </View>
      )}
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    card: { padding: 14 },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 2 },
    filterLabel: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase", marginTop: 4 },
    chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 6 },
    entry: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: 6, marginTop: 2 },
  });
