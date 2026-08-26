import { useCallback, useEffect, useState } from "react";
import { View, Text, ScrollView, TextInput, Pressable, ActivityIndicator, Switch, RefreshControl, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  eveyFetch,
  useAdminSystemServiceGetSystemInfo,
  apiErrorMessage,
  type V1VideoPool,
  type V1VideoRungFit,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

// Mirrors frontend/apps/web/src/routes/_auth/admin/transcoding.lazy.tsx and
// EncodeCalibrationCard.tsx - same endpoints, same fields, same three
// sections (encode calibration, job concurrency, video sessions capacity).
// The web's rich Load-test surface is deliberately NOT ported here; sizing a
// host from a phone screen isn't the workflow, and that panel is large
// enough to warrant its own pass.

type HWFallback = { class: string; codec: string; at: string };

type BenchRungSample = {
  ladder: string;
  height: number;
  fps: number;
  required_fps: number;
  passing: boolean;
  parallel_encodes: number;
};

type EncodeCapability = {
  software_max_height: number;
  software_hdr_max_height?: number;
  hw_max_height: number;
  hw_backend?: string;
  hw_codecs?: string[];
  hw_decode_codecs?: string[];
  hw_device?: string;
  gpu_class?: string;
  hw_mode?: string;
  detected_backends?: string[];
  is_wsl?: boolean;
  benchmarked_at: string;
  benchmark_duration_ms: number;
  hw_fallbacks?: HWFallback[];
  bench_samples?: BenchRungSample[];
};

type SettingValue<T> = { value: T; default: T; source: string };

interface TranscodeServerSettings {
  maxConcurrentTranscodes: SettingValue<number>;
  hwMode: SettingValue<string>;
  av1Enabled: SettingValue<boolean>;
  av1MaxHeight: SettingValue<number>;
}

const CALIBRATION_KEY = ["admin", "transcode-calibration"];
const SETTINGS_KEY = ["admin", "transcoding-settings"];

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

export default function AdminTranscodingScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  const { data: calib, isLoading: calibLoading, refetch: refetchCalib } = useQuery<EncodeCapability>({
    queryKey: CALIBRATION_KEY,
    queryFn: () => eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration", method: "GET" }),
  });

  const { data: settings, isLoading: settingsLoading, refetch: refetchSettings } = useQuery<TranscodeServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<TranscodeServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });

  const { data: sysData, refetch: refetchSys } = useAdminSystemServiceGetSystemInfo();

  // Local mirror of the active HW mode so the picker stays responsive while
  // the detached re-bench runs server-side (the query refetches a few
  // seconds later, same debounce as the web card).
  const [hwMode, setHwModeLocal] = useState("auto");
  useEffect(() => {
    if (calib?.hw_mode) setHwModeLocal(calib.hw_mode);
  }, [calib?.hw_mode]);

  const setHwMode = useMutation({
    mutationFn: (mode: string) =>
      eveyFetch({ url: "/api/v1/admin/server-settings", method: "PUT", data: { hwMode: mode } }),
    onSuccess: (_resp, mode) => {
      setHwModeLocal(mode);
      toast.success(t("admin.calibration.modeApplied", "Mode appliqué - recalibration…"));
      qc.invalidateQueries({ queryKey: CALIBRATION_KEY });
      qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      [3000, 9000, 20000].forEach((ms) => setTimeout(() => qc.invalidateQueries({ queryKey: CALIBRATION_KEY }), ms));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const setAV1 = useMutation({
    mutationFn: (patch: { av1Enabled?: boolean; av1MaxHeight?: number }) =>
      eveyFetch({ url: "/api/v1/admin/server-settings", method: "PUT", data: patch }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.calibration.av1Applied", "Réglage AV1 appliqué"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const recalibrate = useMutation({
    mutationFn: () =>
      eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration/recalibrate", method: "POST" }),
    onSuccess: (fresh) => {
      qc.setQueryData(CALIBRATION_KEY, fresh);
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

  // Job concurrency: Auto (no override) vs a manual cap.
  const [concAuto, setConcAuto] = useState(true);
  const [concManual, setConcManual] = useState("");
  const [concDirty, setConcDirty] = useState(false);
  useEffect(() => {
    if (settings && !concDirty) {
      const overridden = settings.maxConcurrentTranscodes.source === "override";
      setConcAuto(!overridden);
      setConcManual(overridden ? String(settings.maxConcurrentTranscodes.value) : "");
    }
  }, [settings, concDirty]);

  const saveConcurrency = useMutation({
    mutationFn: () =>
      eveyFetch<TranscodeServerSettings>({
        url: "/api/v1/admin/server-settings",
        method: "PUT",
        data: { maxConcurrentTranscodes: concAuto || concManual === "" ? null : parseInt(concManual, 10) },
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      setConcDirty(false);
      toast.success(t("admin.transcoding.saved", "Enregistré"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([refetchCalib(), refetchSettings(), refetchSys()]);
    setRefreshing(false);
  }, [refetchCalib, refetchSettings, refetchSys]);

  const s = styles(theme);
  const isLoading = calibLoading || settingsLoading;

  const av1Enabled = settings?.av1Enabled?.value ?? false;
  const av1MaxHeight = settings?.av1MaxHeight?.value ?? 0;
  const detected = calib?.detected_backends ?? [];
  const isWsl = calib?.is_wsl ?? false;
  const hwEnabled = hwMode !== "off";
  const forceable = Array.from(
    new Set([...detected, ["nvenc", "vaapi", "qsv"].includes(hwMode) ? hwMode : ""].filter(Boolean)),
  );
  const wslBlocked = (b: string) => isWsl && (b === "vaapi" || b === "qsv");
  const fallbacks = calib?.hw_fallbacks ?? [];

  const vs = sysData?.videoSessions;
  const ev = vs?.evidence;
  const pools = (vs?.pools ?? []) as V1VideoPool[];

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

      {isLoading ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <>
          {/* --- Encode calibration / HW ------------------------------------ */}
          <Text style={s.sectionTitle}>{t("admin.calibration.cardTitle", "Calibration d'encodage")}</Text>
          <View style={s.card}>
            <View style={s.switchRow}>
              <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                {t("admin.calibration.hwEnableLabel", "Accélération matérielle")}
              </Text>
              <Switch
                value={hwEnabled}
                onValueChange={() => setHwMode.mutate(hwEnabled ? "off" : "auto")}
                trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                disabled={setHwMode.isPending}
              />
            </View>
            <Text style={s.hint}>
              {hwEnabled
                ? t("admin.calibration.hwEnabledHint", "Encodage sur le GPU quand c'est possible")
                : t("admin.calibration.hwDisabledHint", "Logiciel uniquement (CPU)")}
            </Text>

            {hwEnabled && (
              <>
                <Text style={s.label}>{t("admin.calibration.hwBackendLabel", "Backend")}</Text>
                <View style={s.chipRow}>
                  <Chip
                    theme={theme}
                    label={t("admin.calibration.hwModeAuto", "Auto (meilleur disponible)")}
                    active={hwMode === "auto"}
                    onPress={() => setHwMode.mutate("auto")}
                    disabled={setHwMode.isPending}
                  />
                  {forceable.map((b) => (
                    <Chip
                      key={b}
                      theme={theme}
                      label={b.toUpperCase()}
                      active={hwMode === b}
                      disabled={setHwMode.isPending || wslBlocked(b)}
                      onPress={() => setHwMode.mutate(b)}
                    />
                  ))}
                </View>
                <Text style={s.hint}>
                  {detected.length > 0
                    ? t("admin.calibration.detected", "Détecté : {{list}}", { list: detected.join(", ") })
                    : t("admin.calibration.detectedNone", "Aucun accélérateur GPU détecté")}
                </Text>
                {isWsl && (
                  <Text style={[s.hint, { color: theme.colors.warning }]}>
                    {t(
                      "admin.calibration.wslVaapiNote",
                      "VAAPI/QSV passent par le pilote Mesa d3d12 sous WSL et ne tiennent pas la charge de transcodage ici - utilisez NVENC, ou validez VAAPI sur un hôte Linux natif.",
                    )}
                  </Text>
                )}

                <View style={[s.switchRow, { marginTop: 12 }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                    {t("admin.calibration.av1Label", "Sortie AV1")}
                  </Text>
                  <Switch
                    value={av1Enabled}
                    onValueChange={() => setAV1.mutate({ av1Enabled: !av1Enabled })}
                    trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                    disabled={setAV1.isPending}
                  />
                </View>
                <Text style={s.hint}>
                  {(calib?.hw_codecs ?? []).includes("av1")
                    ? t(
                        "admin.calibration.av1Hint",
                        "Encoder en AV1 quand un client le supporte - meilleure compression. Matériel uniquement ; l'AV1 logiciel n'est jamais utilisé pour le streaming.",
                      )
                    : t(
                        "admin.calibration.av1NoHw",
                        "Aucun encodeur matériel AV1 détecté - activer ceci n'a aucun effet (les flux restent en H.264).",
                      )}
                </Text>

                {av1Enabled && (
                  <>
                    <Text style={s.label}>{t("admin.calibration.av1MaxLabel", "Qualité maximale AV1")}</Text>
                    <View style={s.chipRow}>
                      {[0, 720, 1080, 1440, 2160].map((h) => (
                        <Chip
                          key={h}
                          theme={theme}
                          label={h === 0 ? t("admin.calibration.av1MaxAuto", "Auto (jusqu'à la source)") : `${h}p`}
                          active={av1MaxHeight === h}
                          disabled={setAV1.isPending}
                          onPress={() => setAV1.mutate({ av1MaxHeight: h })}
                        />
                      ))}
                    </View>
                  </>
                )}
              </>
            )}

            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 16 }} />

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

          {/* --- Job concurrency --------------------------------------------- */}
          <Text style={s.sectionTitle}>{t("admin.transcoding.jobConcurrencyTitle", "Concurrence des tâches")}</Text>
          <View style={s.card}>
            <View style={s.switchRow}>
              <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                {t("admin.transcoding.concurrencyAuto", "Détection automatique")}
              </Text>
              <Switch
                value={concAuto}
                onValueChange={(v) => {
                  setConcAuto(v);
                  setConcDirty(true);
                }}
                trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
              />
            </View>
            <Text style={s.hint}>
              {concAuto
                ? t("admin.transcoding.concurrencyAutoHint", "Défaut serveur ({{n}})", {
                    n: settings?.maxConcurrentTranscodes.default ?? 2,
                  })
                : t("admin.transcoding.concurrencyManualHint", "Plafonner les transcodages simultanés")}
            </Text>

            {!concAuto && (
              <TextInput
                value={concManual}
                onChangeText={(v) => {
                  setConcManual(v);
                  setConcDirty(true);
                }}
                style={s.input}
                accessibilityLabel={t("admin.transcoding.concurrencyManualHint", "Plafonner les transcodages simultanés")}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings?.maxConcurrentTranscodes.default ?? 2)}
                keyboardType="numeric"
              />
            )}

            <Pressable
              onPress={() => saveConcurrency.mutate()}
              disabled={!concDirty || saveConcurrency.isPending}
              style={[s.primaryBtn, { opacity: !concDirty || saveConcurrency.isPending ? 0.5 : 1 }]}
            >
              {saveConcurrency.isPending ? (
                <ActivityIndicator size="small" color={theme.colors.onAccent} />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>
              )}
            </Pressable>
          </View>

          {/* --- Video sessions / capacity (read-only) ------------------------ */}
          <Text style={s.sectionTitle}>{t("admin.transcoding.livePlaybackTitle", "Lecture en direct")}</Text>
          <View style={s.card}>
            {vs && (
              <Text style={s.hint}>
                {t("admin.transcoding.videoSessionsAutoAdjusts", "S'ajuste automatiquement au matériel détecté.")}
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
                    <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
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
          </View>
        </>
      )}
    </ScrollView>
  );
}

function Chip({
  theme,
  label,
  active,
  disabled,
  onPress,
}: {
  theme: ReturnType<typeof useTheme>;
  label: string;
  active: boolean;
  disabled?: boolean;
  onPress: () => void;
}) {
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
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
        {label}
      </Text>
    </Pressable>
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
      marginTop: 20,
      marginBottom: 8,
      marginHorizontal: 16,
    },
    card: {
      backgroundColor: theme.colors.surface,
      borderRadius: theme.radii.md,
      marginHorizontal: 16,
      padding: 16,
    },
    switchRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
    label: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase", marginTop: 12, marginBottom: 6 },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 4 },
    chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
    input: { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderRadius: theme.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, marginTop: 8 },
    primaryBtn: { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 16 },
    warnBox: { borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.warning, padding: 10 },
  });
