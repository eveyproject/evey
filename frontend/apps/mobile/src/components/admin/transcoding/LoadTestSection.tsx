import { useState } from "react";
import { View, Text, Pressable, ActivityIndicator, Switch, StyleSheet, TextInput } from "react-native";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, formatBytes, type V1VideoPool } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import {
  ACTIVE_STREAMS_STATUSES,
  BOTTLENECK_REASONS,
  CLIENT_LABEL_KEYS,
  CLIENT_PROFILES,
  CLIENTS_HISTORY,
  DEFAULT_CLIENT_WEIGHTS,
  LOAD_TEST_QUERY_KEY,
  LOAD_TEST_URL,
  STOP_REASON_KEYS,
  clientMixScenario,
  scenarioNoun,
  servedPathLines,
  formatElapsed,
  formatMbps,
  mixParts,
  poolShares,
  predictedStreams,
  type ClientProfile,
  type LoadTestReport,
  type LoadTestScenario,
  type LoadTestStatus,
} from "./loadTest";

// LoadTestSection mirrors the web LoadTestCard + LoadTestReportPanel: throw real
// simulated viewers at the server and report what actually held up (repose ->
// running -> report), with the calibration's prediction shown for a sanity
// check. Same endpoint, same scenarios, same i18n keys as the web.

// The chip for "a mix I state myself": the weights below it decide the
// scenario actually sent.
const CUSTOM_CLIENTS: LoadTestScenario = "clients:custom";
const CLIENT_SCENARIOS: LoadTestScenario[] = [CLIENTS_HISTORY, CUSTOM_CLIENTS];
const USAGE_SCENARIOS: LoadTestScenario[] = ["light", "balanced", "heavy"];
const PURE_SCENARIOS: LoadTestScenario[] = ["transcode", "mkvgo", "natural"];
const SCENARIO_LABEL_KEYS: Record<string, string> = {
  [CLIENTS_HISTORY]: "admin.loadtest.scenarioClientsHistory",
  [CUSTOM_CLIENTS]: "admin.loadtest.scenarioClientsCustom",
  light: "admin.loadtest.scenarioLight",
  balanced: "admin.loadtest.scenarioBalanced",
  heavy: "admin.loadtest.scenarioHeavy",
  transcode: "admin.loadtest.scenarioTranscode",
  mkvgo: "admin.loadtest.scenarioMkvgo",
  natural: "admin.loadtest.scenarioNatural",
};

export function LoadTestSection({ pools }: { pools: V1VideoPool[] }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const s = styles(theme);

  // The audience's own clients are the default: the question an operator has
  // is "how many of MY viewers", and the server decides each one's path.
  const [scenario, setScenario] = useState<LoadTestScenario>(CLIENTS_HISTORY);
  const [weights, setWeights] = useState<Record<ClientProfile, number>>(DEFAULT_CLIENT_WEIGHTS);
  const sentScenario = scenario === CUSTOM_CLIENTS ? clientMixScenario(weights) : scenario;
  const [coldCache, setColdCache] = useState(false);

  const statusQuery = useQuery<LoadTestStatus>({
    queryKey: LOAD_TEST_QUERY_KEY,
    queryFn: () => eveyFetch<LoadTestStatus>({ url: LOAD_TEST_URL, method: "GET" }),
    refetchInterval: (query) => (query.state.data?.running ? 2000 : false),
  });
  const status = statusQuery.data;
  const running = status?.running ?? false;
  const report = status?.lastReport;

  const start = useMutation({
    // No viewer ceiling is sent on purpose: the run climbs until a RESOURCE
    // gives out - a cap here would just cap the answer.
    mutationFn: () =>
      eveyFetch<LoadTestStatus>({ url: LOAD_TEST_URL, method: "POST", data: { scenario: sentScenario, coldCache } }),
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
  const blockedByActiveStreams = !!startError && ACTIVE_STREAMS_STATUSES.includes(startError.httpStatus ?? 0);
  const predicted = predictedStreams(pools);

  return (
    <View style={s.wrap}>
      <Text style={s.heading}>{t("admin.loadtest.heading", "Test de charge")}</Text>
      <Text style={s.hint}>{t("admin.loadtest.idleDescription", "Lance des spectateurs simulés jusqu'à ce qu'une ressource cède.")}</Text>

      {running && status ? (
        <View style={{ gap: 8, marginTop: 8 }}>
          <Text style={{ color: theme.colors.text, fontSize: 14 }}>
            {t("admin.loadtest.runningViewers", { count: status.viewers ?? 0 })}
            {"  ·  "}
            {t("admin.loadtest.runningPhase", { phase: status.phase ?? "" })}
            {"  ·  "}
            {t("admin.loadtest.runningElapsed", { elapsed: formatElapsed(status.elapsedMs ?? 0) })}
          </Text>
          <Pressable
            onPress={() => cancel.mutate()}
            disabled={cancel.isPending}
            style={[s.secondaryBtn, { opacity: cancel.isPending ? 0.5 : 1 }]}
          >
            {cancel.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.text} />
            ) : (
              <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 14 }}>
                {t("admin.loadtest.cancelButton", "Annuler")}
              </Text>
            )}
          </Pressable>
        </View>
      ) : (
        <>
          {report && <ReportPanel report={report} predicted={predicted} />}

          <View style={{ gap: 8, marginTop: 10 }}>
            <Text style={[s.hint, { color: theme.colors.warning }]}>
              {t("admin.loadtest.idleWarning", "Arrêtez toute lecture réelle avant de lancer - le test a besoin d'un hôte au repos.")}
            </Text>
            {blockedByActiveStreams ? (
              <Text style={{ color: theme.colors.danger, fontSize: 13 }}>
                {t("admin.loadtest.errorActiveStreams", "Impossible de lancer : une lecture est déjà en cours.")}
              </Text>
            ) : startError ? (
              <Text style={{ color: theme.colors.danger, fontSize: 13 }}>
                {t("admin.loadtest.errorGeneric", "Le test de charge n'a pas pu démarrer.")}
              </Text>
            ) : null}

            <Text style={s.label}>{t("admin.loadtest.scenarioLabel", "Scénario")}</Text>
            <Text style={s.groupLabel}>{t("admin.loadtest.scenarioGroupClients", "Clients réels")}</Text>
            <View style={s.chipRow}>
              {CLIENT_SCENARIOS.map((sc) => (
                <ScenarioChip key={sc} theme={theme} label={t(SCENARIO_LABEL_KEYS[sc] ?? sc)} active={scenario === sc} disabled={start.isPending} onPress={() => setScenario(sc)} />
              ))}
            </View>
            {scenario === CUSTOM_CLIENTS && (
              <View style={{ gap: 4 }}>
                <Text style={s.hint}>{t("admin.loadtest.clientWeightsLegend", "Part de chaque client (poids relatifs)")}</Text>
                <View style={s.chipRow}>
                  {CLIENT_PROFILES.map((p) => (
                    <View key={p} style={{ gap: 2 }}>
                      <Text style={s.hint}>{t(CLIENT_LABEL_KEYS[p] ?? p)}</Text>
                      <TextInput
                        value={String(weights[p])}
                        keyboardType="number-pad"
                        editable={!start.isPending}
                        onChangeText={(v) => setWeights({ ...weights, [p]: Math.max(0, Number(v) || 0) })}
                        style={{ width: 64, borderWidth: StyleSheet.hairlineWidth, borderColor: theme.colors.border, borderRadius: 6, paddingHorizontal: 8, paddingVertical: 4, color: theme.colors.text }}
                      />
                    </View>
                  ))}
                </View>
              </View>
            )}
            <Text style={s.groupLabel}>{t("admin.loadtest.scenarioGroupUsage", "Profil d'usage")}</Text>
            <View style={s.chipRow}>
              {USAGE_SCENARIOS.map((sc) => (
                <ScenarioChip key={sc} theme={theme} label={t(SCENARIO_LABEL_KEYS[sc] ?? sc)} active={scenario === sc} disabled={start.isPending} onPress={() => setScenario(sc)} />
              ))}
            </View>
            <Text style={s.groupLabel}>{t("admin.loadtest.scenarioGroupPure", "Chemins purs")}</Text>
            <View style={s.chipRow}>
              {PURE_SCENARIOS.map((sc) => (
                <ScenarioChip key={sc} theme={theme} label={t(SCENARIO_LABEL_KEYS[sc] ?? sc)} active={scenario === sc} disabled={start.isPending} onPress={() => setScenario(sc)} />
              ))}
            </View>

            <View style={[s.switchRow, { marginTop: 10 }]}>
              <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1, marginRight: 12 }}>
                {t("admin.loadtest.coldCacheToggle", "Cache froid (mesurer le stockage)")}
              </Text>
              <Switch
                value={coldCache}
                onValueChange={setColdCache}
                disabled={start.isPending}
                trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
              />
            </View>
            <Text style={s.hint}>{t("admin.loadtest.coldCacheHint", "Évince la bibliothèque du cache pour mesurer le disque, pas la RAM.")}</Text>

            <Pressable
              onPress={() => start.mutate()}
              disabled={start.isPending || !sentScenario}
              style={[s.primaryBtn, { opacity: start.isPending || !sentScenario ? 0.5 : 1 }]}
            >
              {start.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.onAccent} />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>
                  {t("admin.loadtest.startButton", "Lancer le test")}
                </Text>
              )}
            </Pressable>
          </View>
        </>
      )}
    </View>
  );
}

function ReportPanel({ report, predicted }: { report: LoadTestReport; predicted: number | undefined }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const s = styles(theme);
  const [showDetails, setShowDetails] = useState(false);

  const isBottleneck = report.stopReason ? BOTTLENECK_REASONS.includes(report.stopReason) : false;

  return (
    <View style={{ gap: 6, paddingTop: 10, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border }}>
      <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "700" }}>
        {t("admin.loadtest.reportHeadline", {
          count: report.sustainedViewers ?? 0,
          scenario: scenarioNoun(t, report.scenario),
        })}
      </Text>
      <Text style={{ color: isBottleneck ? theme.colors.text : theme.colors.textMuted, fontSize: 14, fontWeight: isBottleneck ? "600" : "400" }}>
        {t((report.stopReason && STOP_REASON_KEYS[report.stopReason]) ?? "admin.loadtest.stopReason.error")}
      </Text>
      {report.stopReason === "max_viewers" && (
        <Text style={s.hint}>{t("admin.loadtest.noBottleneckHint", "Aucune ressource n'a cédé : le plafond est ailleurs.")}</Text>
      )}
      {report.coldCache && !report.storageOpaque && (
        <Text style={s.hint}>{t("admin.loadtest.coldCacheLine", "Mesuré cache froid : ce chiffre reflète le stockage, pas la RAM.")}</Text>
      )}
      {report.storageOpaque && (
        <Text style={[s.warnText]}>
          {t(report.coldCache ? "admin.loadtest.storageOpaqueColdLine" : "admin.loadtest.storageOpaqueLine", { fs: report.storageFsType ?? "" })}
        </Text>
      )}
      {report.diskReadStarved && !report.storageOpaque && (
        <Text style={s.warnText}>
          {t(report.coldCache ? "admin.loadtest.coldCacheStillStarvedLine" : "admin.loadtest.servedFromCacheLine")}
        </Text>
      )}
      {(report.storageCollapsesAtReaders ?? 0) > 0 && (
        <Text style={s.warnText}>{t("admin.loadtest.storageCollapseLine", { count: report.storageCollapsesAtReaders ?? 0 })}</Text>
      )}
      <Text style={s.hint}>
        {t("admin.loadtest.ttffLine", { p50: Math.round(report.ttffMsP50 ?? 0), p95: Math.round(report.ttffMsP95 ?? 0) })}
        {"  ·  "}
        {t("admin.loadtest.segmentLine", { p50: Math.round(report.segmentMsP50 ?? 0), p95: Math.round(report.segmentMsP95 ?? 0) })}
      </Text>

      {(report.tiers?.length ?? 0) > 0 && (
        <View style={{ marginTop: 6 }}>
          <View style={[s.tierRow, { borderBottomColor: theme.colors.border }]}>
            <Text style={[s.tierHead, { flex: 1.1 }]}>{t("admin.loadtest.table.viewers", "Spect.")}</Text>
            <Text style={[s.tierHead, { flex: 1 }]}>{t("admin.loadtest.table.cpu", "CPU")}</Text>
            <Text style={[s.tierHead, { flex: 1.4 }]}>{t("admin.loadtest.table.mem", "Mém.")}</Text>
            <Text style={[s.tierHead, { flex: 1.6 }]}>{t("admin.loadtest.table.disk", "Disque")}</Text>
            <Text style={[s.tierHead, { flex: 1.6 }]}>{t("admin.loadtest.table.served", "Servi")}</Text>
          </View>
          {(report.tiers ?? []).map((tier) => {
            const sustained = tier.viewers === report.sustainedViewers;
            const col = sustained ? theme.colors.text : theme.colors.textMuted;
            const weight = sustained ? "700" : "400";
            return (
              <View key={tier.viewers} style={[s.tierRow, { borderBottomColor: theme.colors.border }]}>
                <Text style={[s.tierCell, { flex: 1.1, color: col, fontWeight: weight }]}>{tier.viewers ?? 0}</Text>
                <Text style={[s.tierCell, { flex: 1, color: col, fontWeight: weight }]}>{Math.round(tier.cpuPercent ?? 0)}%</Text>
                <Text style={[s.tierCell, { flex: 1.4, color: col, fontWeight: weight }]}>{formatBytes(tier.memUsedBytes ?? 0)}</Text>
                <Text style={[s.tierCell, { flex: 1.6, color: col, fontWeight: weight }]}>{formatMbps(tier.diskReadBytesPerSec ?? 0)}</Text>
                <Text style={[s.tierCell, { flex: 1.6, color: col, fontWeight: weight }]}>{formatMbps(tier.servedBytesPerSec ?? 0)}</Text>
              </View>
            );
          })}
        </View>
      )}

      {((report.poolBranches?.length ?? 0) > 0 || (report.disks?.length ?? 0) > 0) && (
        <View style={{ gap: 2, marginTop: 6 }}>
          {(report.poolBranches?.length ?? 0) > 0 && (
            <>
              <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.loadtest.poolTitle")}</Text>
              {poolShares(report.poolBranches ?? []).map((b) => (
                <Text key={b.branch} style={s.hint}>
                  {t("admin.loadtest.poolBranchLine", { branch: b.branch, count: b.viewers, pct: b.pct })}
                </Text>
              ))}
              <Text style={s.hint}>{t("admin.loadtest.poolNote")}</Text>
            </>
          )}
          {(report.disks?.length ?? 0) > 0 && (
            <>
              <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600", marginTop: 4 }}>{t("admin.loadtest.disksTitle")}</Text>
              {(report.disks ?? []).map((d) => (
                <Text key={d.device} style={s.hint}>
                  {t("admin.loadtest.diskLine", {
                    device: d.device ?? "",
                    util: Math.round(d.utilPercentAvg ?? 0),
                    peak: Math.round(d.utilPercentPeak ?? 0),
                    rate: formatMbps(d.readBytesPerSecAvg ?? 0),
                    wait: Math.round(d.awaitMsAvg ?? 0),
                  })}
                </Text>
              ))}
              <Text style={s.hint}>{t("admin.loadtest.disksNote")}</Text>
            </>
          )}
        </View>
      )}

      <Pressable onPress={() => setShowDetails((v) => !v)} style={{ marginTop: 6 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {showDetails ? "▾ " : "▸ "}{t("admin.loadtest.detailsSummary", "Détails de métrologie")}
        </Text>
      </Pressable>
      {showDetails && (
        <View style={{ gap: 6, paddingTop: 4 }}>
          {predicted !== undefined && (report.viewerMix?.transcode ?? 0) > 0 && (
            <Text style={s.hint}>{t("admin.loadtest.predictedLine", { count: predicted, measured: report.viewerMix?.transcode ?? 0 })}</Text>
          )}
          {report.viewerMix && Object.keys(report.viewerMix).length > 0 && (
            <Text style={s.hint}>{t("admin.loadtest.viewerMixLine", { parts: mixParts(t, report.viewerMix, "admin.loadtest.mixPart", "count") })}</Text>
          )}
          {servedPathLines(t, report.servedPaths).length > 0 && (
            <View style={{ gap: 2 }}>
              <Text style={s.hint}>{t("admin.loadtest.servedPathsHeading")}</Text>
              {servedPathLines(t, report.servedPaths).map((line) => (
                <Text key={line} style={s.hint}>{"- " + line}</Text>
              ))}
            </View>
          )}
          {report.segmentMsP95ByType && Object.keys(report.segmentMsP95ByType).length > 0 && (
            <Text style={s.hint}>{t("admin.loadtest.segmentByTypeLine", { parts: mixParts(t, report.segmentMsP95ByType, "admin.loadtest.segmentByTypePart", "ms") })}</Text>
          )}
          {report.segmentSloMsByType && Object.keys(report.segmentSloMsByType).length > 0 && (
            <Text style={s.hint}>{t("admin.loadtest.sloByTypeLine", { parts: mixParts(t, report.segmentSloMsByType, "admin.loadtest.segmentByTypePart", "ms") })}</Text>
          )}
          {(report.sharedCacheViewers ?? 0) > 0 && (
            <Text style={s.hint}>{t("admin.loadtest.sharedCacheLine", { count: report.sharedCacheViewers })}</Text>
          )}
          {(report.skippedCandidates ?? 0) > 0 && (
            <Text style={s.hint}>{t("admin.loadtest.skippedCandidatesLine", { count: report.skippedCandidates })}</Text>
          )}
          {report.harnessCpuMeasured && (
            <Text style={s.hint}>{t("admin.loadtest.harnessCpuLine", { pct: Math.round((report.harnessCpuFraction ?? 0) * 100) })}</Text>
          )}
          {(report.resources?.goHeapLiveBytesAvg ?? 0) > 0 && (
            <Text style={s.hint}>
              {t("admin.loadtest.goRuntimeLine", {
                heap: formatBytes(report.resources?.goHeapLiveBytesAvg ?? 0),
                max: formatBytes(report.resources?.goHeapLiveBytesMax ?? 0),
                limit: (report.resources?.goMemLimitBytes ?? 0) > 0 ? formatBytes(report.resources?.goMemLimitBytes ?? 0) : "-",
                gc: Math.round(report.resources?.goGcCpuPercentAvg ?? 0),
                peak: Math.round(report.resources?.goGcCpuPercentPeak ?? 0),
              })}
            </Text>
          )}
        </View>
      )}
    </View>
  );
}

function ScenarioChip({ theme, label, active, disabled, onPress }: { theme: ReturnType<typeof useTheme>; label: string; active: boolean; disabled?: boolean; onPress: () => void }) {
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      style={{
        paddingHorizontal: 12,
        paddingVertical: 6,
        borderRadius: theme.radii.sm,
        borderWidth: 1,
        borderColor: active ? theme.colors.accent : theme.colors.border,
        backgroundColor: active ? theme.colors.accent : "transparent",
        opacity: disabled ? 0.4 : 1,
      }}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>{label}</Text>
    </Pressable>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    wrap: { marginTop: 12, paddingTop: 12, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border },
    heading: { color: theme.colors.text, fontSize: 15, fontWeight: "600" },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 4 },
    label: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase", marginTop: 12 },
    groupLabel: { color: theme.colors.textMuted, fontSize: 12, marginTop: 8, marginBottom: 4 },
    chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
    switchRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
    warnText: { color: theme.colors.warning, fontSize: 13, fontWeight: "600" },
    primaryBtn: { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 12 },
    secondaryBtn: { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md, paddingVertical: 12, alignItems: "center", alignSelf: "flex-start", paddingHorizontal: 20 },
    tierRow: { flexDirection: "row", paddingVertical: 4, borderBottomWidth: StyleSheet.hairlineWidth },
    tierHead: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" },
    tierCell: { fontSize: 12 },
  });
