import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
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
import { toast } from "@evey/store";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { Spinner } from "@/components/ui/Spinner";
import { useDemoLock } from "@/hooks/useDemoMode";

interface Props {
  profileId: string;
  profileName: string;
  open: boolean;
  onClose: () => void;
}

// A kids profile's opening hours: when it may be used. The server enforces
// them on every device; this edits the rules, in two views of the same data.
export function ProfileScheduleDialog({ profileId, profileName, open, onClose }: Props) {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery({
    queryKey: profileScheduleKey(profileId),
    queryFn: ({ signal }) => getProfileSchedule(profileId, signal),
    enabled: open,
  });
  return (
    <Modal open={open} onClose={onClose} title={`${t("auth.profile.schedule.title", "Opening hours")} - ${profileName}`}>
      {isLoading || !data ? (
        <div className="flex justify-center py-8"><Spinner /></div>
      ) : (
        <ScheduleEditor key={profileId} profileId={profileId} state={data} onDone={onClose} />
      )}
    </Modal>
  );
}

function ScheduleEditor({ profileId, state, onDone }: { profileId: string; state: ProfileScheduleState; onDone: () => void }) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { lock } = useDemoLock();
  const initial = state.schedule ?? { ...state.proposal, enabled: false };
  const [enabled, setEnabled] = useState(initial.enabled);
  const [mode, setMode] = useState<ScheduleMode>(initial.mode);
  const [blocks, setBlocks] = useState<ScheduleBlock[]>(() => blocksFor(initial, initial.mode) ?? blocksFor(initial, "daily")!);
  const [modeError, setModeError] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => setModeError(false), [mode]);

  const current = scheduleFromBlocks(enabled, mode, blocks);
  const problems = scheduleProblems({ enabled, mode, rules: blocks.map((b) => ({ days: b.days, ranges: b.ranges })) });

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

  function updateBlock(key: string, ranges: ScheduleRange[]) {
    setBlocks((bs) => bs.map((b) => (b.key === key ? { ...b, ranges } : b)));
  }

  function applyProposal() {
    setEnabled(true);
    setMode(state.proposal.mode);
    setBlocks(blocksFor(state.proposal, state.proposal.mode)!);
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

  const blockLabel = (b: ScheduleBlock) =>
    b.key === "weekdays"
      ? t("auth.profile.schedule.weekdays", "Monday to Friday")
      : b.key === "weekend"
        ? t("auth.profile.schedule.weekend", "Saturday and Sunday")
        : weekdayName(b.days[0], i18n.language);

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-[var(--text-muted)]">
        {t("auth.profile.schedule.description", "Choose when this kids profile can be used. Outside these hours it locks on every device, playback included.")}
      </p>
      <ScheduleStatus state={state} />

      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium text-[var(--text)]">{t("auth.profile.schedule.enable", "Limit to opening hours")}</span>
        <Switch checked={enabled} onChange={() => setEnabled((v) => !v)} label={t("auth.profile.schedule.enable", "Limit to opening hours")} disabled={!!lock.disabled} />
      </div>

      <fieldset disabled={!enabled || !!lock.disabled} className={`flex flex-col gap-4 ${enabled ? "" : "opacity-50"}`}>
        <div role="radiogroup" aria-label={t("auth.profile.schedule.title", "Opening hours")} className="inline-flex self-start rounded-[var(--radius-md)] border border-[var(--border)] p-0.5">
          {(["week", "daily"] as const).map((m) => (
            <button
              key={m}
              type="button"
              role="radio"
              aria-checked={mode === m}
              onClick={() => switchMode(m)}
              className={`px-3 py-1.5 text-sm rounded-[var(--radius-sm)] transition-colors-fast ${mode === m ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "text-[var(--text-muted)] hover:text-[var(--text)]"}`}
            >
              {m === "week" ? t("auth.profile.schedule.modeWeek", "Weekdays / weekend") : t("auth.profile.schedule.modeDaily", "Day by day")}
            </button>
          ))}
        </div>
        {modeError && (
          <p role="alert" className="text-xs" style={{ color: "var(--color-warning)" }}>
            {t("auth.profile.schedule.cannotWeek", "Some days differ from the rest of their block: keep the day-by-day view, or make them alike first.")}
          </p>
        )}

        <div className="flex flex-col gap-3">
          {blocks.map((b) => (
            <BlockEditor key={b.key} label={blockLabel(b)} ranges={b.ranges} onChange={(r) => updateBlock(b.key, r)} />
          ))}
        </div>
      </fieldset>

      {enabled && problems.includes("no_rules") && (
        <p role="alert" className="text-xs" style={{ color: "var(--color-warning)" }}>
          {t("auth.profile.schedule.noHours", "No time range: the profile stays locked at all times.")}
        </p>
      )}
      {problems.includes("empty_range") && (
        <p role="alert" className="text-xs" style={{ color: "var(--color-danger)" }}>
          {t("auth.profile.schedule.emptyRange", "A time range must not start and end at the same time.")}
        </p>
      )}
      <p className="text-xs text-[var(--text-muted)]">
        {t("auth.profile.schedule.timezone", "Times follow the server's time zone ({{zone}}).", { zone: state.timezone })}
      </p>

      <div className="flex flex-wrap justify-between gap-2 pt-2">
        <Button variant="ghost" type="button" onClick={applyProposal} {...lock}>
          {t("auth.profile.schedule.useProposal", "Use the suggested hours")}
        </Button>
        <div className="flex gap-2">
          <Button variant="ghost" type="button" onClick={onDone}>
            {t("common.cancel", "Cancel")}
          </Button>
          <Button type="button" onClick={() => void save()} loading={saving} {...lock} disabled={!!lock.disabled || problems.includes("empty_range")}>
            {t("common.save", "Save")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function BlockEditor({ label, ranges, onChange }: { label: string; ranges: ScheduleRange[]; onChange: (r: ScheduleRange[]) => void }) {
  const { t } = useTranslation();
  const set = (i: number, patch: Partial<ScheduleRange>) => onChange(ranges.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] p-3">
      <div className="flex items-center justify-between gap-2 mb-2">
        <span className="text-sm font-medium text-[var(--text)] first-letter:uppercase">{label}</span>
        {ranges.length === 0 && <span className="text-xs text-[var(--text-muted)]">{t("auth.profile.schedule.dayClosed", "Closed")}</span>}
      </div>
      <ul className="flex flex-col gap-2">
        {ranges.map((r, i) => (
          <li key={i} className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
              {t("auth.profile.schedule.from", "From")}
              <input
                type="time"
                value={minutesToHHMM(r.start)}
                onChange={(e) => {
                  const v = hhmmToMinutes(e.target.value);
                  if (v !== null && v < 1440) set(i, { start: v });
                }}
                className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface)] px-2 py-1 text-sm text-[var(--text)]"
              />
            </label>
            <label className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
              {t("auth.profile.schedule.to", "To")}
              <input
                type="time"
                value={minutesToHHMM(r.end)}
                onChange={(e) => {
                  const v = hhmmToMinutes(e.target.value);
                  if (v !== null) set(i, { end: v === 0 ? 1440 : v });
                }}
                className="rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface)] px-2 py-1 text-sm text-[var(--text)]"
              />
            </label>
            {rangeCrossesMidnight(r) && r.end !== 1440 && (
              <span className="text-xs text-[var(--text-muted)]">{t("auth.profile.schedule.nextDay", "until the next day")}</span>
            )}
            <button
              type="button"
              aria-label={t("auth.profile.schedule.removeRange", "Remove this time range")}
              onClick={() => onChange(ranges.filter((_, j) => j !== i))}
              className="ml-auto p-1.5 rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:text-[var(--color-danger)]"
            >
              <Trash2 size={14} aria-hidden="true" />
            </button>
          </li>
        ))}
      </ul>
      <button
        type="button"
        onClick={() => onChange([...ranges, nextRange(ranges)])}
        className="mt-2 inline-flex items-center gap-1 text-xs text-[var(--accent)] hover:underline"
      >
        <Plus size={12} aria-hidden="true" />
        {t("auth.profile.schedule.addRange", "Add a time range")}
      </button>
    </div>
  );
}

function ScheduleStatus({ state }: { state: ProfileScheduleState }) {
  const { t, i18n } = useTranslation();
  if (!state.schedule?.enabled) return null;
  const a = state.access;
  let text: string;
  if (a.overridden) text = t("auth.profile.schedule.statusOverridden", "Opened by a parent until the next opening");
  else if (a.open && a.closesAt) text = t("auth.profile.schedule.statusOpenUntil", "Open now, until {{time}}", { time: formatTime(a.closesAt, i18n.language) });
  else if (!a.open && a.nextOpen) text = t("auth.profile.schedule.statusClosedUntil", "Locked now, opens {{when}}", { when: formatOpening(a.nextOpen, i18n.language) });
  else text = t("auth.profile.schedule.statusClosed", "Locked now");
  return (
    <p className={`text-sm font-medium ${a.open ? "text-[var(--color-success)]" : "text-[var(--color-warning)]"}`} role="status">
      {text}
    </p>
  );
}
