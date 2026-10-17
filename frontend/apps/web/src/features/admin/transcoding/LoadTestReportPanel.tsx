import { useTranslation } from "react-i18next";
import { formatBytes } from "@evey/api";
import { BOTTLENECK_REASONS, adviceLine, gridRows, deviceLines, formatMbps, limiterLine, mixParts, phaseKey, poolShares, servedPathLines, stopCause, stopReasonKey, verdictLine, windowStateKey } from "./loadTestHelpers";
import type { LoadTestReport, LoadTestResources, LoadTestWindow } from "./loadTestTypes";

// LoadTestReportPanel renders the "answer" of a finished load test run: the
// sustained viewer count, the verdict (what made it stop), and the metrology
// behind both, folded under a <details> so the card stays a few lines and a
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
  const windows = report.windows ?? [];
  // The row the whole run exists to produce: the last hold of the sustained
  // audience that held. Bold, so the eye lands on the answer and not on the
  // search that found it.
  const sustainedRow = windows.map((w, i) => ({ w, i })).filter(({ w }) => w.viewers === report.sustainedViewers && !w.broke).at(-1)?.i;
  // A report from before the mapping existed has no flag and counts as
  // measured; only an explicit false hides the disk figures.
  const diskMeasured = report.diskReadMeasured !== false;
  const res = report.resources;

  const limiter = limiterLine(t, report);
  const advice = adviceLine(t, report);
  const grid = gridRows(report);

  return (
    <div className="flex flex-col gap-1.5" data-testid="load-test-report">
      {/* The main view answers three questions in words an admin who is not
          technical understands: how many, what gives out first, what to do
          about it. Everything else is folded into the technical details. */}
      <p className="text-xl font-bold text-[var(--text)]" data-testid="load-test-verdict">
        {verdictLine(t, report)}
      </p>
      {limiter && (
        <p className="text-sm font-medium text-[var(--text)]" data-testid="load-test-limiter">
          {limiter}
        </p>
      )}
      {advice && (
        <p className="text-sm text-[var(--text)]" data-testid="load-test-advice">
          {advice}
        </p>
      )}
      {/* One row per audience size measured: what it cost and whether it
          held. The repeated windows behind each row are in the details. */}
      {grid.length > 0 && (
        <div className="overflow-x-auto" data-testid="load-test-grid">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--text-muted)]">
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.grid.viewers")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.grid.cpu")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.grid.mem")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.grid.disk")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.grid.served")}</th>
                <th className="py-1 font-medium">{t("admin.loadtest.grid.state")}</th>
              </tr>
            </thead>
            <tbody>
              {grid.map((g, i) => {
                const answer = g.viewers === report.sustainedViewers;
                return (
                  <tr
                    key={`${g.viewers}-${i}`}
                    className={
                      answer
                        ? "border-b border-[var(--border)] font-semibold text-[var(--text)]"
                        : "border-b border-[var(--border)] text-[var(--text-muted)]"
                    }
                    data-testid={answer ? "load-test-grid-answer" : "load-test-grid-row"}
                  >
                    <td className="py-1 pr-3">{g.viewers ?? 0}</td>
                    <td className="py-1 pr-3">{Math.round(g.resources?.cpuPercentAvg ?? 0)}%</td>
                    <td className="py-1 pr-3">{formatBytes(g.resources?.processRssBytesAvg ?? 0)}</td>
                    <td className="py-1 pr-3">{diskMeasured ? formatMbps(g.resources?.diskReadBytesPerSec ?? 0) : "-"}</td>
                    <td className="py-1 pr-3">{formatMbps(g.resources?.servedBytesPerSec ?? 0)}</td>
                    <td className={g.held ? "py-1 text-[var(--color-success)]" : "py-1 text-[var(--color-warning)]"}>
                      {t(g.held ? "admin.loadtest.grid.ok" : "admin.loadtest.grid.ko")}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <details className="text-sm" data-testid="load-test-details">
        <summary className="cursor-pointer select-none text-xs text-[var(--text-muted)] hover:text-[var(--text)]">
          {t("admin.loadtest.detailsSummary")}
        </summary>
        <div className="flex flex-col gap-1.5 pt-2">
      <p
        className={
          report.stopReason && BOTTLENECK_REASONS.includes(report.stopReason)
            ? "text-sm font-medium text-[var(--text)]"
            : "text-sm text-[var(--text-muted)]"
        }
        data-testid="load-test-stop-reason"
      >
        {t(stopReasonKey(report))}
      </p>
      {cause && (
        <p className="text-sm text-[var(--text)]" data-testid="load-test-cause">
          {t(cause.tierState === "broke" ? "admin.loadtest.cause.tierBroke" : "admin.loadtest.cause.tierHeld", { tier: cause.tier })}{" "}
          {t(cause.key, { ...cause.params, what: cause.whatKey ? t(cause.whatKey) : "" })}
        </p>
      )}
      {grid.length > 0 && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-grid-windows">
          {t("admin.loadtest.gridWindowsLine", {
            parts: grid.map((g) => t("admin.loadtest.gridWindowsPart", { viewers: g.viewers ?? 0, passed: g.passed ?? 0, windows: g.windows ?? 0 })).join(", "),
          })}
        </p>
      )}
      {report.stopReason === "max_viewers" && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-no-bottleneck">
          {t("admin.loadtest.noBottleneckHint")}
        </p>
      )}
      {report.endpoint && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-endpoint">
          {t(report.viaProxy ? "admin.loadtest.endpointProxyLine" : "admin.loadtest.endpointDirectLine", { endpoint: report.endpoint })}
        </p>
      )}
      {report.coldCache && !report.storageOpaque && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-cold-cache">
          {t("admin.loadtest.coldCacheLine")}
        </p>
      )}
      {report.storageOpaque && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-storage-opaque">
          {t(report.coldCache ? "admin.loadtest.storageOpaqueColdLine" : "admin.loadtest.storageOpaqueLine", {
            fs: report.storageFsType ?? "",
          })}
        </p>
      )}
      {!diskMeasured && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-disk-unmeasured">
          {t("admin.loadtest.diskUnmeasuredLine")}
        </p>
      )}
      {report.diskReadStarved && !report.storageOpaque && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-disk-starved">
          {t(report.coldCache ? "admin.loadtest.coldCacheStillStarvedLine" : "admin.loadtest.servedFromCacheLine")}
        </p>
      )}
      {(report.storageCollapsesAtReaders ?? 0) > 0 && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-storage-collapse">
          {t("admin.loadtest.storageCollapseLine", {
            count: report.storageCollapsesAtReaders ?? 0,
          })}
        </p>
      )}
      <p className="text-xs text-[var(--text-muted)]">
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
      {/* The machine at the sustained tier: the container's CPU beside the
          whole machine's, the time the cores spent waiting on the storage,
          and the container's memory beside the server process alone. */}
      {res && (res.samples ?? 0) > 0 && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-machine">
          {t("admin.loadtest.machineLine", {
            tier: report.sustainedViewers ?? 0,
            cpu: Math.round(res.cpuPercentAvg ?? 0),
            machine: Math.round(res.machineCpuPercentAvg ?? 0),
            iowait: Math.round(res.machineIowaitPercentAvg ?? 0),
            mem: formatBytes(res.memUsedBytesAvg ?? 0),
            rss: formatBytes(res.processRssBytesAvg ?? 0),
          })}
        </p>
      )}

      {/* One row per HOLD, broken ones included and marked: the climb as it
          happened. Each row carries the raw p95 and the p95 the stall verdict
          was read on (scaled to the reference segment). The disk column is
          the machine's block counters over the library's devices, absent
          when none was mapped. */}
      {windows.length > 0 && (
        <div className="overflow-x-auto" data-testid="load-test-windows">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--text-muted)]">
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.viewers")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.state")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.cpu")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.iowait")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.mem")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.disk")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.served")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.p95Raw")}</th>
                <th className="py-1 font-medium">{t("admin.loadtest.table.p95Verdict")}</th>
              </tr>
            </thead>
            <tbody>
              {windows.map((w, i) => {
                const sustained = i === sustainedRow;
                return (
                  <tr
                    key={i}
                    className={
                      sustained
                        ? "border-b border-[var(--border)] font-semibold text-[var(--text)]"
                        : w.broke
                          ? "border-b border-[var(--border)] text-[var(--color-warning)]"
                          : "border-b border-[var(--border)] text-[var(--text-muted)]"
                    }
                    data-testid={sustained ? "load-test-tier-sustained" : w.broke ? "load-test-window-broke" : "load-test-window"}
                  >
                    <td className="py-1 pr-3">{w.viewers ?? 0}</td>
                    <td className="py-1 pr-3">
                      {t(windowStateKey(w))} · {t(phaseKey(w.phase))}
                    </td>
                    <td className="py-1 pr-3">{cpuCell(w.resources)}</td>
                    <td className="py-1 pr-3">{Math.round(w.resources?.machineIowaitPercentAvg ?? 0)}%</td>
                    <td className="py-1 pr-3">{memCell(w.resources)}</td>
                    <td className="py-1 pr-3">{diskMeasured ? formatMbps(w.resources?.diskReadBytesPerSec ?? 0) : t("admin.loadtest.notMeasured")}</td>
                    <td className="py-1 pr-3">{formatMbps(w.resources?.servedBytesPerSec ?? 0)}</td>
                    <td className="py-1 pr-3">{Math.round(w.segmentMsP95 ?? 0)} ms</td>
                    <td className="py-1">{Math.round(w.segmentMsP95Scaled ?? 0)} ms</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="text-xs text-[var(--text-muted)]">{t("admin.loadtest.table.note")}</p>
        </div>
      )}
      {/* A report from before the holds were kept: one row per measured tier. */}
      {windows.length === 0 && (report.tiers?.length ?? 0) > 0 && (
        <div className="overflow-x-auto" data-testid="load-test-tiers">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)] text-left text-xs text-[var(--text-muted)]">
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.viewers")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.cpu")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.mem")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.loadtest.table.disk")}</th>
                <th className="py-1 font-medium">{t("admin.loadtest.table.served")}</th>
              </tr>
            </thead>
            <tbody>
              {(report.tiers ?? []).map((tier) => {
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
                    <td className="py-1 pr-3">
                      {Math.round(tier.cpuPercent ?? 0)}% · {Math.round(tier.machineCpuPercent ?? 0)}%
                    </td>
                    <td className="py-1 pr-3">
                      {formatBytes(tier.memUsedBytes ?? 0)} · {formatBytes(tier.processRssBytes ?? 0)}
                    </td>
                    <td className="py-1 pr-3">{diskMeasured ? formatMbps(tier.diskReadBytesPerSec ?? 0) : t("admin.loadtest.notMeasured")}</td>
                    <td className="py-1">{formatMbps(tier.servedBytesPerSec ?? 0)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {/* What ONE more viewer costs: the slope between measured tiers, never
          a total divided by a headcount (that charges the idle server to
          the viewers). */}
      {report.perViewerCost && (report.perViewerCost.samples ?? 0) > 0 && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-per-viewer">
          {t("admin.loadtest.perViewerLine", {
            cpu: (report.perViewerCost.cpuPercentAvg ?? 0).toFixed(2),
            mem: formatBytes(report.perViewerCost.memUsedBytesAvg ?? 0),
            disk: diskMeasured ? formatMbps(report.perViewerCost.diskReadBytesPerSec ?? 0) : t("admin.loadtest.notMeasured"),
            net: formatMbps(report.perViewerCost.servedBytesPerSec ?? 0),
          })}
        </p>
      )}
      {/* The capacity at the break, per disk: the headcount depends on the
          draw, what the disks delivered when they gave out does not. */}
      {(report.breakViewers ?? 0) > 0 && (report.breakDisks?.length ?? 0) > 0 && diskMeasured && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-break-capacity">
          {t("admin.loadtest.breakCapacityLine", {
            tier: report.breakViewers ?? 0,
            parts: (report.breakDisks ?? [])
              .map((d) => t("admin.loadtest.breakCapacityPart", { device: d.device ?? "", rate: formatMbps(d.readBytesPerSecAvg ?? 0), peak: formatMbps(d.readBytesPerSecPeak ?? 0) }))
              .join(", "),
          })}
        </p>
      )}
      {/* A pooled mount: where the retained tier's audience sat (a pool is
          not a RAID, a file lives on ONE disk), what each physical drive did,
          and which device each location was read from - resolved at the
          run's start, because letters rotate at reboot. */}
      {((report.poolBranches?.length ?? 0) > 0 || (report.disks?.length ?? 0) > 0 || (report.diskDevices?.length ?? 0) > 0) && (
        <div className="flex flex-col gap-1" data-testid="load-test-pool">
          {(report.poolBranches?.length ?? 0) > 0 && (
            <>
              <p className="text-xs font-medium text-[var(--text)]">{t("admin.loadtest.poolTitle", { tier: report.sustainedViewers ?? 0 })}</p>
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
              <p className="pt-1 text-xs font-medium text-[var(--text)]">{t("admin.loadtest.disksTitle", { tier: report.sustainedViewers ?? 0 })}</p>
              {(report.disks ?? []).map((d) => (
                <p key={d.device} className="text-xs text-[var(--text-muted)]" data-testid="load-test-disk">
                  {t("admin.loadtest.diskLine", {
                    device: d.device ?? "",
                    util: Math.round(d.utilPercentAvg ?? 0),
                    peak: Math.round(d.utilPercentPeak ?? 0),
                    rate: formatMbps(d.readBytesPerSecAvg ?? 0),
                    ratePeak: formatMbps(d.readBytesPerSecPeak ?? 0),
                    wait: Math.round(d.awaitMsAvg ?? 0),
                    waitPeak: Math.round(d.awaitMsPeak ?? 0),
                    queue: (d.queueDepthAvg ?? 0).toFixed(1),
                  })}
                </p>
              ))}
              <p className="text-xs text-[var(--text-muted)]">{t("admin.loadtest.disksNote")}</p>
            </>
          )}
          {(report.diskDevices?.length ?? 0) > 0 && (
            <>
              <p className="pt-1 text-xs font-medium text-[var(--text)]">{t("admin.loadtest.devicesTitle")}</p>
              {deviceLines(t, report.diskDevices).map((line) => (
                <p key={line} className="text-xs text-[var(--text-muted)]" data-testid="load-test-device">
                  {line}
                </p>
              ))}
            </>
          )}
        </div>
      )}
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
          {/* Counted once per viewer over the WHOLE climb: a cumulative
              figure, labelled as one. */}
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
          {report.segmentSloMsByType && Object.keys(report.segmentSloMsByType).length > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-slo-by-type">
              {t("admin.loadtest.sloByTypeLine", {
                parts: mixParts(t, report.segmentSloMsByType, "admin.loadtest.segmentByTypePart", "ms"),
              })}
            </p>
          )}
          {/* Viewers of the retained tier placed on a title another viewer
              already read, at another position. No cache serves them: the
              packager builds every segment from the source. */}
          {(report.sharedCacheViewers ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-replayed">
              {t("admin.loadtest.replayedTitlesLine", { count: report.sharedCacheViewers })}
            </p>
          )}
          {(report.skippedCandidates ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-skipped">
              {t("admin.loadtest.skippedCandidatesLine", { count: report.skippedCandidates })}
            </p>
          )}
          {/* What the packager asked its sources for, counted in-process -
              or that nothing counted it. */}
          {res && (res.samples ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-app-read">
              {report.appReadMeasured
                ? t("admin.loadtest.appReadLine", {
                    rate: formatMbps(res.appReadBytesPerSec ?? 0),
                    ratio: (report.appReadAmplification ?? 0).toFixed(2),
                  })
                : t("admin.loadtest.appReadUnmeasuredLine")}
            </p>
          )}
          {report.harnessCpuMeasured && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-harness-cpu">
              {t("admin.loadtest.harnessCpuLine", {
                pct: Math.round((report.harnessCpuFraction ?? 0) * 100),
              })}
            </p>
          )}
          {(res?.goHeapLiveBytesAvg ?? 0) > 0 && (
            <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-go-runtime">
              {t("admin.loadtest.goRuntimeLine", {
                heap: formatBytes(res?.goHeapLiveBytesAvg ?? 0),
                max: formatBytes(res?.goHeapLiveBytesMax ?? 0),
                limit: (res?.goMemLimitBytes ?? 0) > 0 ? formatBytes(res?.goMemLimitBytes ?? 0) : "-",
                gc: Math.round(res?.goGcCpuPercentAvg ?? 0),
                peak: Math.round(res?.goGcCpuPercentPeak ?? 0),
              })}
            </p>
          )}
        </div>
      </details>
    </div>
  );
}

// cpuCell is "container · machine", both whole percentages.
function cpuCell(r: LoadTestResources | undefined): string {
  return `${Math.round(r?.cpuPercentAvg ?? 0)}% · ${Math.round(r?.machineCpuPercentAvg ?? 0)}%`;
}

// memCell is "container · server process".
function memCell(r: LoadTestResources | undefined): string {
  return `${formatBytes(r?.memUsedBytesAvg ?? 0)} · ${formatBytes(r?.processRssBytesAvg ?? 0)}`;
}

export type { LoadTestWindow };
