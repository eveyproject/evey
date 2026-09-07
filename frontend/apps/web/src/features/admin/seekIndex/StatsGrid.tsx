import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import type { SeekIndexPageState } from "./hooks";

type StatsGridProps = Pick<
  SeekIndexPageState,
  "result" | "triage" | "breakdown" | "withBackup" | "ackGroup" | "libraryId" | "busy"
> & { result: NonNullable<SeekIndexPageState["result"]> };

// The grid IS the triage, and the triage is two cards wide: what Evey repairs
// here, and what it merely noticed. Each carries the action that belongs to it
// (repair lives on the button above; acknowledge lives here) and the per-defect
// breakdown underneath, so a number never appears without saying what it is
// made of. There is deliberately no third card handing the operator homework.
//
// The two triage cards are given twice the width of the plain counters: their
// breakdown is a list of six phrases, and squeezed into a quarter of the row it
// ran straight out of the card.
export function StatsGrid({ result, triage, breakdown, withBackup, ackGroup, libraryId, busy }: StatsGridProps) {
  const { t } = useTranslation();
  // Each part is its own item in a wrapping list rather than a run of inline
  // text: a phrase like "47 aux pistes audio divergentes" must be free to sit
  // on its own line, and `break-words` is the last resort for the narrowest
  // viewport where even one phrase is wider than the card.
  const part = (count: number, key: string, testId: string) =>
    count > 0 ? (
      <li key={testId} data-testid={testId} className="break-words">
        {t(key, { count })}
      </li>
    ) : null;
  const partList = (parts: ReactNode[]) => {
    const present = parts.filter(Boolean);
    return (
      <ul className="mt-1 flex flex-col gap-0.5 text-xs text-[var(--text-muted)]">
        {present.length === 0 ? <li>{t("admin.seekIndex.breakdownNone")}</li> : present}
      </ul>
    );
  };
  const ackAll = () =>
    ackGroup.mutate({ data: { ...(libraryId ? { libraryId } : {}), verdict: "informational" } });

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-6 gap-4 items-start">
      <Card className="min-w-0">
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">{t("admin.seekIndex.statsInspected")}</dt>
        <dd className="text-xl font-bold text-[var(--text)]">{result.total ?? 0}</dd>
      </Card>
      <Card className="min-w-0 lg:col-span-2">
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">{t("admin.seekIndex.statsRepairable")}</dt>
        <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-repairable-count">
          {triage.repairable}
        </dd>
        {partList([
          part(breakdown.missing, "admin.seekIndex.partMissing", "seek-index-missing-count"),
          part(breakdown.damaged, "admin.seekIndex.partDamaged", "seek-index-damaged-count"),
          part(breakdown.wrongContainer, "admin.seekIndex.partWrongContainer", "seek-index-wrong-container-count"),
          part(breakdown.resyncRequired, "admin.seekIndex.partResyncRequired", "seek-index-resync-required-count"),
        ])}
      </Card>
      <Card className="min-w-0 lg:col-span-2">
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">{t("admin.seekIndex.statsInformational")}</dt>
        <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-informational-count">
          {triage.informational}
        </dd>
        {partList([
          part(breakdown.truncated, "admin.seekIndex.partTruncated", "seek-index-truncated-count"),
          part(breakdown.seekCoarse, "admin.seekIndex.partSeekCoarse", "seek-index-coarse-count"),
          part(breakdown.tracksDisagree, "admin.seekIndex.partTracksDisagree", "seek-index-tracks-disagree-count"),
          part(breakdown.delayedAudio, "admin.seekIndex.partAudioDelay", "seek-index-audio-delay-count"),
          part(breakdown.audioShort, "admin.seekIndex.partAudioShort", "seek-index-audio-short-count"),
          part(breakdown.trailingJunk, "admin.seekIndex.partTrailingJunk", "seek-index-trailing-junk-count"),
        ])}
        <p className="mt-2 text-xs text-[var(--text-muted)]">{t("admin.seekIndex.statsInformationalHint")}</p>
        {triage.informational > 0 && (
          <Button
            variant="ghost"
            className="mt-2 text-xs"
            disabled={busy || ackGroup.isPending}
            onClick={ackAll}
            data-testid="seek-index-ack-informational-btn"
          >
            {t("admin.seekIndex.ackGroupBtn", { count: triage.informational })}
          </Button>
        )}
      </Card>
      <Card className="min-w-0">
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">{t("admin.seekIndex.statsBackup")}</dt>
        <dd className="text-xl font-bold text-[var(--text)]">{withBackup}</dd>
        {triage.acked > 0 && (
          <p className="mt-1 text-xs text-[var(--text-muted)]" data-testid="seek-index-acked-count">
            {t("admin.seekIndex.partAcked", { count: triage.acked })}
          </p>
        )}
      </Card>
    </div>
  );
}
