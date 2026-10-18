import { useEffect, useState } from "react";
import { ChevronDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, useAdminSystemServiceGetSystemInfo } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { InfoTooltip } from "@/components/ui/InfoTooltip";
import { LoadTestReportPanel } from "./LoadTestReportPanel";
import {
  ACTIVE_STREAMS_STATUSES,
  CLIENT_LABEL_KEYS,
  CLIENT_PROFILES,
  CLIENTS_HISTORY,
  DEFAULT_CLIENT_WEIGHTS,
  LOAD_TEST_QUERY_KEY,
  LOAD_TEST_URL,
  clientMixScenario,
  formatElapsed,
  liveElapsedMs,
  phaseKey,
  predictedStreams,
} from "./loadTestHelpers";
import type { ClientProfile, LoadTestScenario, LoadTestStatus } from "./loadTestTypes";

// The select's value for "a mix I state myself": the weights below it decide
// the scenario actually sent.
const CUSTOM_CLIENTS = "clients:custom";

// LoadTestCard is the "constat, not a control panel" companion to
// VideoSessionsCard: instead of trusting the calibration's prediction, it
// throws real simulated viewers at the server and reports what actually held
// up. Repose (launch) -> running (poll) -> report, with the calibration's own
// prediction shown alongside the measured number for a sanity check.
export function LoadTestCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: sysData } = useAdminSystemServiceGetSystemInfo();
  const pools = sysData?.videoSessions?.pools ?? [];
  // The audience's own clients answer the question that actually matters to an
  // operator - "how many of MY viewers can this host hold" - so they are the
  // default: the server decides each viewer's path as it does for real.
  const [scenario, setScenario] = useState<LoadTestScenario>(CLIENTS_HISTORY);
  const [weights, setWeights] = useState<Record<ClientProfile, number>>(DEFAULT_CLIENT_WEIGHTS);
  const sentScenario = scenario === CUSTOM_CLIENTS ? clientMixScenario(weights) : scenario;
  // Cold cache: keep evicting the library from the page cache so the run
  // measures the storage, not the RAM. Off by default - the warm figure is
  // today's truth on this box; cold is the day the library outgrows memory.
  const [coldCache, setColdCache] = useState(false);

  const statusQuery = useQuery<LoadTestStatus>({
    queryKey: LOAD_TEST_QUERY_KEY,
    queryFn: () => eveyFetch<LoadTestStatus>({ url: LOAD_TEST_URL, method: "GET" }),
    refetchInterval: (query) => (query.state.data?.running ? 2000 : false),
  });
  const status = statusQuery.data;
  const running = status?.running ?? false;
  const report = status?.lastReport;
  // The counter ticks every second between polls (liveElapsedMs).
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!running) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [running]);

  const start = useMutation({
    // No viewer ceiling is sent, and that is the point: the run climbs until a
    // RESOURCE gives out. A number picked here would just cap the answer, and
    // the whole question is how far this machine actually goes.
    mutationFn: () =>
      eveyFetch<LoadTestStatus>({
        url: LOAD_TEST_URL,
        method: "POST",
        data: { scenario: sentScenario, coldCache },
      }),
    onSuccess: (data) => {
      qc.setQueryData(LOAD_TEST_QUERY_KEY, data);
      void qc.invalidateQueries({ queryKey: LOAD_TEST_QUERY_KEY });
    },
  });

  const cancel = useMutation({
    mutationFn: () => eveyFetch<LoadTestStatus>({ url: LOAD_TEST_URL, method: "DELETE" }),
    onSuccess: (data) => {
      qc.setQueryData(LOAD_TEST_QUERY_KEY, data);
      void qc.invalidateQueries({ queryKey: LOAD_TEST_QUERY_KEY });
    },
  });

  const startError = start.error as (Error & { httpStatus?: number }) | null;
  const blockedByActiveStreams =
    !!startError && ACTIVE_STREAMS_STATUSES.includes(startError.httpStatus ?? 0);

  const predicted = predictedStreams(pools);

  return (
    <div className="flex flex-col gap-3 pt-3 mt-2 border-t border-[var(--border)]" data-testid="load-test-card">
      {/* The what-does-this-measure explanation lives behind the info button,
          not as standing paragraphs: it is read once, the card is used often. */}
      <h4 className="text-sm font-semibold text-[var(--text)] flex items-center gap-1">
        {t("admin.loadtest.heading")}
        <InfoTooltip label={t("admin.loadtest.heading")}>
          <span className="block">{t("admin.loadtest.idleDescription")}</span>
          <span className="block mt-1.5">{t("admin.loadtest.noCeilingLine")}</span>
        </InfoTooltip>
      </h4>

      {running && status ? (
        <div className="flex flex-col gap-2" data-testid="load-test-running">
          <p className="text-sm text-[var(--text)]">
            {t("admin.loadtest.runningViewers", { count: status.viewers ?? 0 })}
            {" · "}
            {t("admin.loadtest.runningPhase", { phase: t(phaseKey(status.phase)) })}
            {" · "}
            {t("admin.loadtest.runningElapsed", { elapsed: formatElapsed(liveElapsedMs(status.elapsedMs, statusQuery.dataUpdatedAt, now)) })}
          </p>
          <div className="flex justify-start">
            <Button
              variant="secondary"
              loading={cancel.isPending}
              onClick={() => cancel.mutate()}
              data-testid="load-test-cancel-btn"
            >
              {t("admin.loadtest.cancelButton")}
            </Button>
          </div>
        </div>
      ) : (
        <>
          {report && <LoadTestReportPanel report={report} predicted={predicted} />}

          <div className="flex flex-col gap-2" data-testid="load-test-idle">
            <p className="text-xs text-[var(--color-warning)]">{t("admin.loadtest.idleWarning")}</p>

            {blockedByActiveStreams ? (
              <p className="text-sm text-[var(--color-danger)]" role="alert" data-testid="load-test-error">
                {t("admin.loadtest.errorActiveStreams")}
              </p>
            ) : startError ? (
              <p className="text-sm text-[var(--color-danger)]" role="alert" data-testid="load-test-error">
                {t("admin.loadtest.errorGeneric")}
              </p>
            ) : null}

            <div className="flex flex-wrap items-end gap-3">
              {/* Native <select> with <optgroup> - the shared Select component
                  doesn't support groups, and grouping "usage profile" (the
                  question that matters: how many different humans) from the
                  3 "pure" single-path lanes is the whole point here. */}
              <div className="flex flex-col gap-1 w-64">
                <label htmlFor="load-test-scenario" className="text-sm font-medium text-[var(--text-muted)]">
                  {t("admin.loadtest.scenarioLabel")}
                </label>
                <div className="relative">
                  <select
                    id="load-test-scenario"
                    value={scenario}
                    onChange={(e) => setScenario(e.target.value as LoadTestScenario)}
                    disabled={start.isPending}
                    data-testid="load-test-scenario-select"
                    className="w-full appearance-none px-3 py-2 pr-8 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] text-base sm:text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <optgroup label={t("admin.loadtest.scenarioGroupClients")}>
                      <option value={CLIENTS_HISTORY}>{t("admin.loadtest.scenarioClientsHistory")}</option>
                      <option value={CUSTOM_CLIENTS}>{t("admin.loadtest.scenarioClientsCustom")}</option>
                    </optgroup>
                    <optgroup label={t("admin.loadtest.scenarioGroupUsage")}>
                      <option value="light">{t("admin.loadtest.scenarioLight")}</option>
                      <option value="balanced">{t("admin.loadtest.scenarioBalanced")}</option>
                      <option value="heavy">{t("admin.loadtest.scenarioHeavy")}</option>
                    </optgroup>
                    <optgroup label={t("admin.loadtest.scenarioGroupPure")}>
                      <option value="transcode">{t("admin.loadtest.scenarioTranscode")}</option>
                      <option value="mkvgo">{t("admin.loadtest.scenarioMkvgo")}</option>
                      <option value="natural">{t("admin.loadtest.scenarioNatural")}</option>
                    </optgroup>
                  </select>
                  <ChevronDown
                    className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)]"
                    size={16}
                  />
                </div>
              </div>
              <Button
                loading={start.isPending}
                disabled={!sentScenario}
                onClick={() => start.mutate()}
                data-testid="load-test-start-btn"
              >
                {t("admin.loadtest.startButton")}
              </Button>
            </div>
            {scenario === CUSTOM_CLIENTS && (
              <fieldset className="flex flex-wrap gap-3" data-testid="load-test-client-weights">
                <legend className="text-xs text-[var(--text-muted)] mb-1">{t("admin.loadtest.clientWeightsLegend")}</legend>
                {CLIENT_PROFILES.map((p) => (
                  <label key={p} className="flex flex-col gap-1 text-xs text-[var(--text-muted)]">
                    {t(CLIENT_LABEL_KEYS[p] ?? p)}
                    <input
                      type="number"
                      min={0}
                      max={100}
                      value={weights[p]}
                      disabled={start.isPending}
                      onChange={(e) => setWeights({ ...weights, [p]: Math.max(0, Number(e.target.value) || 0) })}
                      data-testid={`load-test-weight-${p}`}
                      className="w-20 px-2 py-1 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] text-sm"
                    />
                  </label>
                ))}
              </fieldset>
            )}
            {/* Cold cache: measure the storage instead of the RAM. On a box
                whose library fits in memory, the default (warm) run overstates
                the viewer count - its own report warns about it. */}
            <Checkbox
              label={t("admin.loadtest.coldCacheToggle")}
              checked={coldCache}
              onChange={setColdCache}
              disabled={start.isPending}
              data-testid="load-test-cold-cache-toggle"
            />
            <p className="text-xs text-[var(--text-muted)]">{t("admin.loadtest.coldCacheHint")}</p>
          </div>
        </>
      )}
    </div>
  );
}
