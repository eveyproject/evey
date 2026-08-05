import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import type { SeekIndexPageState } from "./hooks";

interface StatsGridProps {
  result: NonNullable<SeekIndexPageState["result"]>;
  missing: SeekIndexPageState["missing"];
  withBackup: SeekIndexPageState["withBackup"];
  delayedAudio: SeekIndexPageState["delayedAudio"];
  wrongContainer: SeekIndexPageState["wrongContainer"];
  trailingJunk: SeekIndexPageState["trailingJunk"];
  audioShort: SeekIndexPageState["audioShort"];
  seekCoarse: SeekIndexPageState["seekCoarse"];
}

export function StatsGrid({
  result,
  missing,
  withBackup,
  delayedAudio,
  wrongContainer,
  trailingJunk,
  audioShort,
  seekCoarse,
}: StatsGridProps) {
  const { t } = useTranslation();

  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
      <Card>
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
          {t("admin.seekIndex.statsInspected")}
        </dt>
        <dd className="text-xl font-bold text-[var(--text)]">{result.total ?? 0}</dd>
      </Card>
      <Card>
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
          {t("admin.seekIndex.statsMissing")}
        </dt>
        <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-missing-count">
          {missing}
        </dd>
      </Card>
      <Card>
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
          {t("admin.seekIndex.statsBackup")}
        </dt>
        <dd className="text-xl font-bold text-[var(--text)]">{withBackup}</dd>
      </Card>
      <Card>
        <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
          {t("admin.seekIndex.statsAudioDelay")}
        </dt>
        <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-audio-delay-count">
          {delayedAudio}
        </dd>
      </Card>
      {wrongContainer > 0 && (
        <Card>
          <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
            {t("admin.seekIndex.statsWrongContainer")}
          </dt>
          <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-wrong-container-count">
            {wrongContainer}
          </dd>
        </Card>
      )}
      {trailingJunk > 0 && (
        <Card>
          <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
            {t("admin.seekIndex.statsTrailingJunk")}
          </dt>
          <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-trailing-junk-count">
            {trailingJunk}
          </dd>
        </Card>
      )}
      {audioShort > 0 && (
        <Card>
          <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
            {t("admin.seekIndex.statsAudioShort")}
          </dt>
          <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-audio-short-count">
            {audioShort}
          </dd>
        </Card>
      )}
      {seekCoarse > 0 && (
        <Card>
          <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
            {t("admin.seekIndex.statsSeekCoarse")}
          </dt>
          <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-coarse-count">
            {seekCoarse}
          </dd>
        </Card>
      )}
    </div>
  );
}
