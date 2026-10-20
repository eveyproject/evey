import { useCallback, useState } from "react";
import { View, Text, ScrollView, Pressable, ActivityIndicator, RefreshControl, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  eveyFetch,
  useAdminSystemServiceGetSystemInfo,
  getServerConfigServiceListServerConfigEntriesQueryKey,
  type V1VideoPool,
  type V1VideoRungFit,
  type V1VideoSourceFit,
} from "@evey/api";
import { toast } from "@evey/store";
import { SERVER_SETTINGS_KEY, useServerSettings, type ServerSettingField } from "@evey/client";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadTestSection } from "../../../src/components/admin/transcoding/LoadTestSection";
import { CALIBRATION_KEY, useCalibration, type EncodeCapability } from "../../../src/components/admin/transcoding/calibration";
import { EncodeSettingsGroup } from "../../../src/components/admin/transcoding/EncodeSettingsGroup";
import { CapacitySettingsGroup } from "../../../src/components/admin/transcoding/CapacitySettingsGroup";
import { ServerSettingsGroup } from "../../../src/components/admin/settings/ServerSettingsGroup";

// Mirrors frontend/apps/web/src/routes/_auth/admin/transcoding.lazy.tsx: what
// the admin SETS apart from what the server MEASURED. Settings first
// (encoding, capacity, cache, then the expert limits), each group with its
// own Save or applied at once; diagnostics below, read-only apart from
// "Recalibrate" and the load test.

const ADVANCED_FIELDS: ServerSettingField[] = [
  "transcodeMaxSessions",
  "softwareMaxHeight",
  "softwareHDRMaxHeight",
  "softwareAV1MaxHeight",
  "transcodeSingleVariant",
  "transcodeHWPerVariant",
];

// codecLabel uppercases the wire codec for display ("h264" -> "H264").
function codecLabel(codec?: string): string {
  return (codec ?? "").toUpperCase();
}

// sourceFits answers "how many can I serve" for the rung load actually lands on
// - the CHEAPEST one, the same rung the count cap is derived from (mirror web
// VideoSessionsCard). Reading the top rung would quote a ceiling nobody hits.
function sourceFits(rungs: V1VideoRungFit[]): V1VideoSourceFit[] {
  let cheapest: V1VideoRungFit | undefined;
  for (const r of rungs) {
    if (!r.bySource?.length) continue;
    if (!cheapest || (r.cost ?? 0) < (cheapest.cost ?? 0)) cheapest = r;
  }
  return cheapest?.bySource ?? [];
}

// poolRungFits picks the ONE ladder class each pool is best described by -
// "software" for the cpu pool (falling back to whatever else is there), and
// the (only) "hw" class for the gpu pool.
function poolRungFits(pool: V1VideoPool): V1VideoRungFit[] {
  const classes = pool.classes ?? [];
  const primary = classes.find((c) => c.ladderClass === "software") ?? classes[0];
  return primary?.rungFits ?? [];
}

function fallbackReasonKey(t: ReturnType<typeof useTranslation>["t"], cls: string): string {
  switch (cls) {
    case "hw_unsupported_option":
      return t("admin.calibration.fallbackUnsupportedOption", "option encodeur non supportée");
    case "hw_encoder_missing":
      return t("admin.calibration.fallbackEncoderMissing", "encodeur indisponible");
    case "hw_init":
      return t("admin.calibration.fallbackInit", "échec d'initialisation GPU");
    default:
      return cls;
  }
}

function SectionHeading({ id, title, desc }: { id: string; title: string; desc: string }) {
  const theme = useTheme();
  return (
    <View style={[headingStyles.wrap, { borderBottomColor: theme.colors.border }]} testID={`transcoding-${id}-heading`}>
      <Text style={[headingStyles.title, { color: theme.colors.text }]} accessibilityRole="header">
        {title}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 2 }}>{desc}</Text>
    </View>
  );
}

// AdvancedLimitsGroup: the expert ceilings for weak hardware, most read at
// boot (each row says so through its restart badge), the software height
// ceilings and the AV1 software ceiling offered as the ladder the server
// accepts.
function AdvancedLimitsGroup() {
  const { t } = useTranslation();
  const theme = useTheme();
  const { data } = useServerSettings();
  const needsRestart = ADVANCED_FIELDS.some((f) => data?.[f]?.requiresRestart);
  return (
    <ServerSettingsGroup
      title={t("admin.transcoding.advancedTitle", "Advanced limits")}
      description={t("admin.transcoding.advancedSubtitle")}
      fields={ADVANCED_FIELDS}
      saveTestID="advanced-transcoding-save-btn"
      testID="group-advanced-limits"
      before={
        needsRestart ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, paddingVertical: 10 }} testID="advanced-restart-hint">
            {t("admin.transcoding.advancedRestartHint")}
          </Text>
        ) : undefined
      }
    />
  );
}

export default function AdminTranscodingScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  const { data: calib, isLoading: calibLoading, refetch: refetchCalib } = useCalibration();
  const { data: sysData, refetch: refetchSys } = useAdminSystemServiceGetSystemInfo();

  const recalibrate = useMutation({
    mutationFn: () =>
      eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration/recalibrate", method: "POST" }),
    onSuccess: () => {
      // The POST answers with the bare capability; the GET wraps it with
      // hw_mode, is_wsl and hw_fallbacks. Writing the POST body into the cache
      // blanked those until the next refetch, so re-read the GET instead.
      qc.invalidateQueries({ queryKey: CALIBRATION_KEY });
      toast.success(t("admin.calibration.successToast", "Recalibration terminée"));
    },
    onError: (err: Error) => {
      if (/transcodes[_ ]active/i.test(err.message)) {
        toast.error(
          t(
            "admin.calibration.transcodesActiveToast",
            "Arrêtez la lecture en cours avant de recalibrer - le benchmark a besoin d'un hôte au repos pour mesurer le vrai plafond.",
          ),
        );
      } else if (/409|already in progress/i.test(err.message)) {
        toast.error(t("admin.calibration.busyToast", "Recalibration déjà en cours"));
      } else {
        toast.error(t("admin.calibration.errorToast", "Échec de la recalibration"));
      }
    },
  });

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([
      refetchCalib(),
      refetchSys(),
      qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY }),
      qc.invalidateQueries({ queryKey: getServerConfigServiceListServerConfigEntriesQueryKey() }),
    ]);
    setRefreshing(false);
  }, [refetchCalib, refetchSys, qc]);

  const s = styles(theme);
  const fallbacks = calib?.hw_fallbacks ?? [];
  const vs = sysData?.videoSessions;
  const ev = vs?.evidence;
  const pools = (vs?.pools ?? []) as V1VideoPool[];
  const benchSource = vs?.benchSource;
  const decodeRates = vs?.decodeThroughput ?? [];

  const fmtDate = (iso: string) => {
    try {
      return new Date(iso).toLocaleString(i18n.language);
    } catch {
      return iso;
    }
  };

  return (
    <ScrollView
      keyboardShouldPersistTaps="handled"
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("admin.transcoding.title", "Configuration de l'encodage"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {calibLoading ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <View style={{ padding: 16 }}>
          <View testID="transcoding-settings">
            <SectionHeading id="settings" title={t("admin.settingsLayout.transcodingSettings")} desc={t("admin.settingsLayout.transcodingSettingsDesc")} />
            <EncodeSettingsGroup />
            <CapacitySettingsGroup />
            <ServerSettingsGroup
              title={t("admin.settingsLayout.groupCache")}
              description={t("admin.settingsLayout.groupCacheDesc")}
              fields={["transcodeCacheMaxGB"]}
              saveTestID="server-settings-save-cache"
              testID="group-cache"
            />
            <AdvancedLimitsGroup />
          </View>

          <View testID="transcoding-diagnostics">
            <SectionHeading id="diagnostics" title={t("admin.settingsLayout.transcodingDiagnostics")} desc={t("admin.settingsLayout.transcodingDiagnosticsDesc")} />

            {/* --- Encode calibration (measured) ---------------------------- */}
            <Text style={s.sectionTitle}>{t("admin.calibration.cardTitle", "Calibration d'encodage")}</Text>
            <View style={s.card}>
              <Text style={[s.hint, { marginTop: 0 }]}>{t("admin.calibration.cardSubtitle")}</Text>
              <Row theme={theme} label={t("admin.calibration.softwareMax", "Plafond d'encodage logiciel")}>
                {calib ? `${calib.software_max_height}p (${t("admin.calibration.libx264", "libx264 veryfast")})` : "-"}
              </Row>
              {calib?.software_hdr_max_height ? (
                <Row theme={theme} label={t("admin.calibration.softwareHdrMax", "Plafond HDR logiciel")}>
                  {`${calib.software_hdr_max_height}p (${t("admin.calibration.hdrTonemap", "tonemap HDR→SDR")})`}
                </Row>
              ) : null}
              <Row theme={theme} label={t("admin.calibration.hwMax", "Plafond d'encodage matériel")}>
                {calib && calib.hw_max_height > 0
                  ? `${calib.hw_max_height}p (${calib.hw_backend ?? ""}${
                      calib.gpu_class === "integrated"
                        ? `, ${t("admin.calibration.gpuIntegrated", "iGPU")}`
                        : calib.gpu_class === "discrete"
                          ? `, ${t("admin.calibration.gpuDiscrete", "GPU dédié")}`
                          : ""
                    })`
                  : t("admin.calibration.hwNone", "aucun accélérateur GPU détecté")}
              </Row>
              {calib?.benchmarked_at ? (
                <Row theme={theme} label={t("admin.calibration.lastRun", "Dernière calibration")}>
                  {`${fmtDate(calib.benchmarked_at)} (${(calib.benchmark_duration_ms / 1000).toFixed(1)}s)`}
                </Row>
              ) : null}

              {fallbacks.length > 0 && (
                <View style={[s.warnBox, { marginTop: 12 }]}>
                  <Text style={{ color: theme.colors.warning, fontSize: 13, fontWeight: "600" }}>
                    {t("admin.calibration.hwFallbacks", "Bascules matérielles")}
                  </Text>
                  {fallbacks
                    .slice()
                    .reverse()
                    .map((f, i) => (
                      <Text key={i} style={{ color: theme.colors.text, fontSize: 12, marginTop: 4 }}>
                        {(f.codec || "h264").toUpperCase()} → software · {fallbackReasonKey(t, f.class)} ·{" "}
                        <Text style={{ color: theme.colors.textMuted }}>{fmtDate(f.at)}</Text>
                      </Text>
                    ))}
                </View>
              )}

              <Pressable
                onPress={() => recalibrate.mutate()}
                disabled={recalibrate.isPending}
                accessibilityRole="button"
                testID="recalibrate-btn"
                style={[s.primaryBtn, { opacity: recalibrate.isPending ? 0.5 : 1 }]}
              >
                {recalibrate.isPending ? (
                  <ActivityIndicator size="small" color={theme.colors.onAccent} />
                ) : (
                  <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>
                    {t("admin.calibration.recalibrate", "Recalibrer")}
                  </Text>
                )}
              </Pressable>
              <Text style={[s.hint, { textAlign: "center", marginTop: 8 }]}>
                {t(
                  "admin.calibration.recalibrateHint",
                  "Relancez le benchmark après avoir activé l'accélération matérielle, changé de CPU, ou migré le conteneur vers un autre hôte.",
                )}
              </Text>
            </View>

            {/* --- Live playback (read-only) + load test ---------------------- */}
            <Text style={s.sectionTitle}>{t("admin.transcoding.livePlaybackTitle", "Lecture en direct")}</Text>
            <View style={s.card}>
              <Text style={[s.hint, { marginTop: 0 }]}>{t("admin.transcoding.livePlaybackSubtitle")}</Text>
              {vs && (
                <Text style={s.hint}>
                  {t("admin.transcoding.videoSessionsAutoAdjusts", "S'ajuste automatiquement au matériel détecté.")}
                </Text>
              )}

              {(benchSource?.codec || decodeRates.length > 0) && (
                <Text style={s.hint}>
                  {benchSource?.codec
                    ? t("admin.transcoding.videoBenchSource", {
                        codec: codecLabel(benchSource.codec),
                        height: benchSource.height,
                      })
                    : ""}
                  {decodeRates.length > 0 && benchSource?.codec ? " · " : ""}
                  {decodeRates
                    .map((d) => t("admin.transcoding.videoDecodeRate", { codec: codecLabel(d.codec), fps: Math.round(d.fps ?? 0) }))
                    .join(" · ")}
                </Text>
              )}

              {pools.length > 0 ? (
                pools.map((pool) => {
                  const used = pool.budgetUsed ?? 0;
                  const total = pool.budgetTotal ?? 0;
                  const pct = total > 0 ? Math.min(100, Math.max(0, (used / total) * 100)) : 0;
                  const rungs = poolRungFits(pool);
                  const label =
                    pool.pool === "gpu"
                      ? t("admin.transcoding.videoPoolGpu", "Pool GPU")
                      : t("admin.transcoding.videoPoolCpu", "Pool CPU");
                  return (
                    <View key={pool.pool} style={{ marginTop: 8 }}>
                      <View style={{ flexDirection: "row", justifyContent: "space-between", alignItems: "center" }}>
                        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{label}</Text>
                        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                          {t("admin.transcoding.videoPoolBudget", "{{used}} / {{total}}", {
                            used: Math.round(used * 100) / 100,
                            total: Math.round(total * 100) / 100,
                          })}
                        </Text>
                      </View>
                      <View style={{ height: 6, borderRadius: 3, backgroundColor: theme.colors.border, marginTop: 4 }}>
                        <View
                          style={{
                            height: 6,
                            borderRadius: 3,
                            width: `${pct}%`,
                            backgroundColor: theme.colors.accent,
                          }}
                        />
                      </View>
                      {rungs.map((r) => (
                        <Text key={r.height} style={[s.hint, { marginTop: 2 }]}>
                          {t("admin.transcoding.videoPoolRungFit", "{{count}} flux à {{height}}p", {
                            count: r.maxStreams ?? 1,
                            height: r.height,
                          })}
                        </Text>
                      ))}
                      {sourceFits(rungs).length > 0 && (
                        <View style={{ marginTop: 8, borderTopWidth: 1, borderTopColor: theme.colors.border, paddingTop: 8 }}>
                          <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>
                            {t("admin.transcoding.videoSourceTitle", "Par source")}
                          </Text>
                          {sourceFits(rungs).map((f) => (
                            <View
                              key={`${f.source?.codec}-${f.source?.height}`}
                              style={{ flexDirection: "row", justifyContent: "space-between", marginTop: 2 }}
                            >
                              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                                {codecLabel(f.source?.codec)} {f.source?.height}p
                              </Text>
                              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                                {t("admin.transcoding.videoSourceStreams", { count: f.maxStreams ?? 1 })}
                              </Text>
                            </View>
                          ))}
                        </View>
                      )}
                    </View>
                  );
                })
              ) : (
                <>
                  {vs && (
                    <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>
                      {t("admin.transcoding.videoSessionsMeasured", "Auto : {{auto}} - actifs : {{active}}", {
                        auto: vs.autoCapacity,
                        active: vs.active,
                      })}
                    </Text>
                  )}
                  {ev && ev.derived !== false && (
                    <Text style={s.hint}>
                      {t(
                        "admin.transcoding.videoSessionsEvidence",
                        "{{height}}p à {{fps}} im/s (besoin {{requiredFps}}), {{cores}} cœurs",
                        {
                          height: ev.height,
                          fps: Math.round((ev.fps ?? 0) * 10) / 10,
                          requiredFps: Math.round((ev.requiredFps ?? 0) * 10) / 10,
                          cores: ev.cpuBudget,
                        },
                      )}
                    </Text>
                  )}
                  {ev && ev.derived === false && (
                    <Text style={[s.hint, { color: theme.colors.warning }]}>
                      {t("admin.transcoding.videoSessionsNotMeasured", "Pas encore mesuré")}
                    </Text>
                  )}
                </>
              )}
              {/* Load test: real simulated viewers, same section as the web's
                  Live-playback card (VideoSessionsCard + LoadTestCard). */}
              <LoadTestSection pools={pools} />
            </View>
          </View>
        </View>
      )}
    </ScrollView>
  );
}

function Row({ theme, label, children }: { theme: ReturnType<typeof useTheme>; label: string; children: React.ReactNode }) {
  return (
    <View style={{ marginTop: 8 }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, textTransform: "uppercase" }}>{label}</Text>
      <Text style={{ color: theme.colors.text, fontSize: 14, marginTop: 2 }}>{children}</Text>
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    sectionTitle: {
      color: theme.colors.textMuted,
      fontSize: 13,
      fontWeight: "600",
      textTransform: "uppercase",
      marginTop: 4,
      marginBottom: 8,
    },
    card: {
      backgroundColor: theme.colors.surface,
      borderRadius: theme.radii.md,
      marginBottom: 16,
      padding: 16,
    },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 4 },
    primaryBtn: { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 16 },
    warnBox: { borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.warning, padding: 10 },
  });

const headingStyles = StyleSheet.create({
  wrap: { borderBottomWidth: StyleSheet.hairlineWidth, paddingBottom: 8, marginBottom: 16, marginTop: 8 },
  title: { fontSize: 18, fontWeight: "700" },
});
