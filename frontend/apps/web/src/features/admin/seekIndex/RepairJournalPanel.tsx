import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAdminMediaServiceListMediaRepairs, baseName } from "@evey/api";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { repairOpLabel, repairEvidenceLine } from "@evey/client";

// What the repairs touched, across the whole library.
//
// The 98 files rewritten on the night of 2026-07-13 were found seven weeks
// later by a hand-written query on file modification times, because nothing
// else recorded the batch. This panel is that question asked of the journal:
// a window, a batch kind, and every file a repair changed in it - with the
// evidence each action rested on, so a rewrite made on a bare declaration
// stands out from one made on a measurement.

type Window = "24h" | "7d" | "30d" | "all";
const WINDOW_MS: Record<Window, number> = { "24h": 86_400_000, "7d": 7 * 86_400_000, "30d": 30 * 86_400_000, all: 0 };
const WINDOWS: Window[] = ["24h", "7d", "30d", "all"];
const WINDOW_KEYS: Record<Window, string> = {
  "24h": "admin.repairJournal.window24h",
  "7d": "admin.repairJournal.window7d",
  "30d": "admin.repairJournal.window30d",
  all: "admin.repairJournal.windowAll",
};

// "operator" is a client-side view: entries with no batch, i.e. a person
// acting on one file (an undo, a shift judged on a preview clip).
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


function outcomeVariant(outcome: string | undefined): "success" | "warning" | "error" | "default" {
  switch (outcome) {
    case "applied":
      return "success";
    case "rolled_back":
      return "warning";
    case "failed":
      return "error";
    default:
      return "default";
  }
}


export function RepairJournalPanel() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [window, setWindow] = useState<Window>("7d");
  const [job, setJob] = useState<JobFilter>("");

  // Pinned to the moment the filter changed, NOT recomputed on every render:
  // this value goes into the query key, so a fresh Date.now() each render made
  // every render a new key, every new key a new fetch, and the panel sat on
  // "loading" for ever while refetching itself.
  const sinceMs = useMemo(
    () => (WINDOW_MS[window] > 0 ? String(Date.now() - WINDOW_MS[window]) : undefined),
    [window],
  );
  const journal = useAdminMediaServiceListMediaRepairs(
    { ...(sinceMs ? { sinceMs } : {}), ...(job && job !== "operator" ? { job } : {}), limit: 200 },
    { query: { enabled: open } },
  );
  const entries = (journal.data?.entries ?? []).filter((e) => job !== "operator" || !e.job);

  return (
    <Card data-testid="repair-journal">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("admin.repairJournal.title")}</h3>
          <p className="text-xs text-[var(--text-muted)]">{t("admin.repairJournal.intro")}</p>
        </div>
        <Button variant="ghost" className="text-xs" onClick={() => setOpen((v) => !v)} data-testid="repair-journal-toggle">
          {open ? t("admin.repairJournal.hideBtn") : t("admin.repairJournal.showBtn")}
        </Button>
      </div>

      {open && (
        <div className="mt-3 space-y-2">
          <div className="flex flex-wrap items-center gap-3 text-xs">
            <label className="flex items-center gap-1 text-[var(--text-muted)]">
              {t("admin.repairJournal.windowLabel")}
              <select
                className="rounded border border-[var(--border)] bg-[var(--surface)] px-1 py-0.5 text-[var(--text)]"
                value={window}
                onChange={(ev) => setWindow(ev.target.value as Window)}
                data-testid="repair-journal-window"
              >
                {WINDOWS.map((w) => (
                  <option key={w} value={w}>
                    {t(WINDOW_KEYS[w])}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center gap-1 text-[var(--text-muted)]">
              {t("admin.repairJournal.jobLabel")}
              <select
                className="rounded border border-[var(--border)] bg-[var(--surface)] px-1 py-0.5 text-[var(--text)]"
                value={job}
                onChange={(ev) => setJob(ev.target.value as JobFilter)}
                data-testid="repair-journal-job"
              >
                {JOBS.map((j) => (
                  <option key={j} value={j}>
                    {t(JOB_KEYS[j])}
                  </option>
                ))}
              </select>
            </label>
            {!journal.isLoading && !journal.isError && (
              <span className="text-[var(--text-muted)]" data-testid="repair-journal-count">
                {t("admin.repairJournal.count", { count: entries.length })}
              </span>
            )}
          </div>

          {journal.isLoading && <p className="text-xs text-[var(--text-muted)]">{t("common.loading")}</p>}
          {journal.isError && <FetchErrorState error={journal.error} onRetry={() => void journal.refetch()} />}
          {!journal.isLoading && !journal.isError && entries.length === 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="repair-journal-empty">
              {t("admin.repairJournal.empty")}
            </p>
          )}
          {entries.length > 0 && (
            <ul className="space-y-1.5">
              {entries.map((e, i) => (
                <li key={i} className="text-xs text-[var(--text-muted)]" data-testid="repair-journal-entry">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="text-[var(--text)]">{new Date(Number(e.atMs ?? 0)).toLocaleString()}</span>
                    <span className="font-mono text-[var(--text)]" title={e.path}>
                      {baseName(e.path ?? "")}
                    </span>
                    <Badge variant="info">{repairOpLabel(e.op, t)}</Badge>
                    <Badge variant={outcomeVariant(e.outcome)}>{t(`admin.repairHistory.outcome.${e.outcome}`)}</Badge>
                    {e.job && <span>{t(JOB_KEYS[(e.job as JobFilter) in JOB_KEYS ? (e.job as JobFilter) : ""])}</span>}
                  </div>
                  <div>{repairEvidenceLine(e, t)}</div>
                  {e.error && <div className="text-[var(--error)]">{e.error}</div>}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Card>
  );
}
