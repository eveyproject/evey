import { useTranslation } from "react-i18next";
import { formatBytes } from "@evey/api";
import { BOTTLENECK_REASONS, STOP_REASON_KEYS, formatMbps, stopCause, mixParts, poolShares, scenarioNoun, servedPathLines } from "./loadTestHelpers";
import type { LoadTestReport } from "./loadTestTypes";

// LoadTestReportPanel renders the "answer" of a finished load test run: the
// sustained viewer count, the verdict (what made it stop), and the metrology
// behind both, folded under a <details> so the card stays three lines and a
// table for the operator who only wants the headline.
export function LoadTestReportPanel({
  report,
  predicted,
}: {
  report: LoadTestReport;
  predicted: number | undefined;
}) {
  const { t } = useTranslation();
  const cause = stopCause(report);

  return (
    <div className="flex flex-col gap-1.5" data-testid="load-test-report">
      {/* The answer, then the verdict, then anything that changes how to
          READ the answer (cold cache, blind disk, collapsing storage).
          Everything else is metrology and lives folded below the table:
          an operator sizing a machine needs three lines, not fifteen. */}
      <p className="text-xl font-bold text-[var(--text)]">
        {t("admin.loadtest.reportHeadline", {
          count: report.sustainedViewers ?? 0,
          scenario: scenarioNoun(t, report.scenario),
        })}
      </p>
      {/* The verdict. When a resource gave out, THAT is the answer the
          operator came for - the viewer count above is only its
          consequence. stopDetail stays in the logs: it is the server's
          own diagnostic string, English and technical. */}
      <p
        className={
          report.stopReason && BOTTLENECK_REASONS.includes(report.stopReason)
            ? "text-sm font-medium text-[var(--text)]"
            : "text-sm text-[var(--text-muted)]"
        }
        data-testid="load-test-stop-reason"
      >
        {t(
          (report.stopReason && STOP_REASON_KEYS[report.stopReason]) ??
            "admin.loadtest.stopReason.error",
        )}
      </p>
      {/* The cause, in plain words: what the server failed on, or which
          resource gave out, or that none did - so the verdict answers
          "why" without opening the logs. */}
      {cause && (
        <p className="text-sm font-medium text-[var(--text)]" data-testid="load-test-cause">
          {t(cause.key, { ...cause.params, what: cause.whatKey ? t(cause.whatKey) : "" })}
        </p>
      )}
      {report.stopReason === "max_viewers" && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-no-bottleneck">
          {t("admin.loadtest.noBottleneckHint")}
        </p>
      )}
      {/* A cold-cache run measured the storage on purpose: say so, or
          its (lower) figure gets compared against a warm run's as if
          the server had regressed. */}
      {report.coldCache && !report.storageOpaque && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-cold-cache">
          {t("admin.loadtest.coldCacheLine")}
        </p>
      )}
      {/* An opaque mount (mergerfs, NFS, 9p...) hides the disks from the
          server's kernel. Said once, up front, in place of the cold-cache and
          silent-disk lines: it is the one explanation behind both, and
          "measured the storage" would not be true there. */}
      {report.storageOpaque && (
        <p className="text-sm font-medium text-[var(--color-warning)]" data-testid="load-test-storage-opaque">
          {t(report.coldCache ? "admin.loadtest.storageOpaqueColdLine" : "admin.loadtest.storageOpaqueLine", {
            fs: report.storageFsType ?? "",
          })}
        </p>
      )}
      {/* Same signal, two different truths. Warm run: the disk sat idle
          because the RAM served everything - the count is overstated.
          Cold run: the cache WAS being evicted, so a silent disk means
          the kernel cannot see the reads at all (a network/9p mount, or
          a cache outside this OS - a VM host, a NAS controller). */}
      {report.diskReadStarved && !report.storageOpaque && (
        <p className="text-sm font-medium text-[var(--color-warning)]" data-testid="load-test-disk-starved">
          {t(report.coldCache ? "admin.loadtest.coldCacheStillStarvedLine" : "admin.loadtest.servedFromCacheLine")}
        </p>
      )}
      {/* Storage that SERIALISES is invisible to every gauge and quietly
          decides how many people can watch. It stays: it is not a detail,
          it is a hardware verdict. */}
      {(report.storageCollapsesAtReaders ?? 0) > 0 && (
        <p className="text-sm font-medium text-[var(--color-warning)]" data-testid="load-test-storage-collapse">
          {t("admin.loadtest.storageCollapseLine", {
            count: report.storageCollapsesAtReaders ?? 0,
          })}
        </p>
      )}
      <p className="text-sm text-[var(--text-muted)]">
        {t("admin.loadtest.ttffLine", {
          p50: Math.round(report.ttffMsP50 ?? 0),
          p95: Math.round(report.ttffMsP95 ?? 0),
        })}
        {" · "}
        {t("admin.loadtest.segmentLine", {
          p50: Math.round(report.segmentMsP50 ?? 0),
          p95: Math.round(report.segmentMsP95 ?? 0),
        })}
      </p>

      {/* What those viewers actually COST the box, one row per audience
          size the run measured - the climb itself, not just the rung it
          stopped on. A single headline figure cannot say WHY the run
          stopped where it did; the trend down the per-viewer columns can:
          it either holds steady (the server was still scaling, and the
          ceiling is elsewhere) or it bends upward (the server was starting
          to struggle, and the ceiling is real).

          Everything finer - read amplification, the storage curve, the
          per-disk breakdown - lives in the logs. It belongs to whoever is
          debugging the server, not to whoever is sizing it. */}
      {(report.tiers?.length ?? 0) > 0 && (
        <div className="overflow-x-auto" data-testid="load-test-tiers">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--text-muted)]">
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.viewers")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.cpu")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.mem")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.disk")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.served")}</th>
                <th className="py-1 font-medium">{t("admin.loadtest.table.perViewer")}</th>
              </tr>
            </thead>
            <tbody>
              {(report.tiers ?? []).map((tier) => {
                /* The row the whole run exists to produce: the audience the
                   server actually sustained. Bold, so the eye lands on the
                   answer and not on the search that found it. */
                const sustained = tier.viewers === report.sustainedViewers;
                return (
                  <tr
                    key={tier.viewers}
                    className={
                      sustained
                        ? "border-b border-[var(--border)] font-semibold text-[var(--text)]"
                        : "border-b border-[var(--border)] text-[var(--text-muted)]"
                    }
                    data-testid={sustained ? "load-test-tier-sustained" : undefined}
                  >
                    <td className="py-1 pr-3">{tier.viewers ?? 0}</td>
                    <td className="py-1 pr-3">{Math.round(tier.cpuPercent ?? 0)}%</td>
                    <td className="py-1 pr-3">{formatBytes(tier.memUsedBytes ?? 0)}</td>
                    <td className="py-1 pr-3">{formatMbps(tier.diskReadBytesPerSec ?? 0)}</td>
                    <td className="py-1 pr-3">{formatMbps(tier.servedBytesPerSec ?? 0)}</td>
                    <td className="py-1">
                      {t("admin.loadtest.table.perViewerCell", {
                        cpu: (tier.cpuPercentPerViewer ?? 0).toFixed(2),
                        mem: formatBytes(tier.memBytesPerViewer ?? 0),
                      })}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {/* A pooled mount (mergerfs) shows the kernel one disk where there are
          several. Two things decide such a run and neither is visible from
          inside the mount: where the audience's titles sit (a pool is not a
          RAID, a file lives on ONE disk) and how busy each physical drive was.
          Shown beside the table, not folded: it is the answer to "why this
          number". */}
      {((report.poolBranches?.length ?? 0) > 0 || (report.disks?.length ?? 0) > 0) && (
        <div className="flex flex-col gap-1" data-testid="load-test-pool">
          {(report.poolBranches?.length ?? 0) > 0 && (
            <>
              <p className="text-xs font-medium text-[var(--text)]">{t("admin.loadtest.poolTitle")}</p>
              {poolShares(report.poolBranches ?? []).map((b) => (
                <p key={b.branch} className="text-xs text-[var(--text-muted)]" data-testid="load-test-pool-branch">
                  {t("admin.loadtest.poolBranchLine", { branch: b.branch, count: b.viewers, pct: b.pct })}
                </p>
              ))}
              <p className="text-xs text-[var(--text-muted)]">{t("admin.loadtest.poolNote")}</p>
            </>
          )}
          {(report.disks?.length ?? 0) > 0 && (
            <>
              <p className="pt-1 text-xs font-medium text-[var(--text)]">{t("admin.loadtest.disksTitle")}</p>
              {(report.disks ?? []).map((d) => (
                <p key={d.device} className="text-xs text-[var(--text-muted)]" data-testid="load-test-disk">
                  {t("admin.loadtest.diskLine", {
                    device: d.device ?? "",
                    util: Math.round(d.utilPercentAvg ?? 0),
                    peak: Math.round(d.utilPercentPeak ?? 0),
                    rate: formatMbps(d.readBytesPerSecAvg ?? 0),
                    wait: Math.round(d.awaitMsAvg ?? 0),
                  })}
                </p>
              ))}
              <p className="text-xs text-[var(--text-muted)]">{t("admin.loadtest.disksNote")}</p>
            </>
          )}
        </div>
      )}
      {/* Without this, every CPU figure in the table is a lie by a factor
          of several: the simulated viewers run inside the server's own
          cgroup, and the kernel bills their HTTP overhead to the server.
          The run measures that share and subtracts it - and says so, because
          an operator who does not know the correction was applied cannot
          trust the number he is sizing his hardware on. */}
      {/* The metrology: real, but secondary. Per-path breakdowns, the
          calibration cross-check, the harness correction - the lines an
          operator reads once and a debugger reads often. Folded, so the
          card stays three lines and a table. (A <details> keeps its
          content in the DOM, so nothing here hides from tests or a
          ctrl-F.) */}
      <details className="text-sm" data-testid="load-test-details">
        <summary className="cursor-pointer select-none text-xs text-[var(--text-muted)] hover:text-[var(--text)]">
          {t("admin.loadtest.detailsSummary")}
        </summary>
        <div className="flex flex-col gap-1.5 pt-2">
          {predicted !== undefined && (report.viewerMix?.transcode ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-predicted">
              {t("admin.loadtest.predictedLine", {
                count: predicted,
                measured: report.viewerMix?.transcode ?? 0,
              })}
            </p>
          )}
          {report.viewerMix && Object.keys(report.viewerMix).length > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-viewer-mix">
              {t("admin.loadtest.viewerMixLine", {
                parts: mixParts(t, report.viewerMix, "admin.loadtest.mixPart", "count"),
              })}
            </p>
          )}
          {/* What the server decided for each kind of viewer: the same
              audience costs a different server depending on who could copy
              what, so a client-mix verdict names it. */}
          {servedPathLines(t, report.servedPaths).length > 0 && (
            <div className="text-xs text-[var(--text-muted)]" data-testid="load-test-served-paths">
              <p>{t("admin.loadtest.servedPathsHeading")}</p>
              <ul className="list-disc pl-4">
                {servedPathLines(t, report.servedPaths).map((line) => (
                  <li key={line}>{line}</li>
                ))}
              </ul>
            </div>
          )}
          {report.segmentMsP95ByType && Object.keys(report.segmentMsP95ByType).length > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-segment-by-type">
              {t("admin.loadtest.segmentByTypeLine", {
                parts: mixParts(t, report.segmentMsP95ByType, "admin.loadtest.segmentByTypePart", "ms"),
              })}
            </p>
          )}
          {/* Each path is judged against its OWN segment length: mkvgo
              packages on keyframe boundaries, the ffmpeg ladder emits 3s
              segments. A single shared budget would measure one path with
              the other's yardstick - and the looser one always wins. */}
          {report.segmentSloMsByType && Object.keys(report.segmentSloMsByType).length > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-slo-by-type">
              {t("admin.loadtest.sloByTypeLine", {
                parts: mixParts(t, report.segmentSloMsByType, "admin.loadtest.segmentByTypePart", "ms"),
              })}
            </p>
          )}
          {(report.sharedCacheViewers ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]">
              {t("admin.loadtest.sharedCacheLine", { count: report.sharedCacheViewers })}
            </p>
          )}
          {(report.skippedCandidates ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-skipped">
              {t("admin.loadtest.skippedCandidatesLine", { count: report.skippedCandidates })}
            </p>
          )}
          {/* Without this, every CPU figure in the table is a lie by a
              factor of several: the simulated viewers run inside the
              server's own cgroup. The run measures that share and
              subtracts it - and says so. */}
          {report.harnessCpuMeasured && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-harness-cpu">
              {t("admin.loadtest.harnessCpuLine", {
                pct: Math.round((report.harnessCpuFraction ?? 0) * 100),
              })}
            </p>
          )}
          {/* Whether a slow run was the disk or the garbage collector: GOMEMLIMIT
              is soft, so a heap near it makes the collector run back to back and
              slows the whole server in a way that looks like a storage ceiling. */}
          {(report.resources?.goHeapLiveBytesAvg ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-go-runtime">
              {t("admin.loadtest.goRuntimeLine", {
                heap: formatBytes(report.resources?.goHeapLiveBytesAvg ?? 0),
                max: formatBytes(report.resources?.goHeapLiveBytesMax ?? 0),
                limit: (report.resources?.goMemLimitBytes ?? 0) > 0 ? formatBytes(report.resources?.goMemLimitBytes ?? 0) : "-",
                gc: Math.round(report.resources?.goGcCpuPercentAvg ?? 0),
                peak: Math.round(report.resources?.goGcCpuPercentPeak ?? 0),
              })}
            </p>
          )}
        </div>
      </details>
    </div>
  );
}
