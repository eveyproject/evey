import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Switch } from "@/components/ui/Switch";

// EncodeCapability is the JSON shape served by the backend at
// /api/v1/admin/transcode/calibration. Mirrors the Go domain struct.
type HWFallback = { class: string; codec: string; at: string };

// BenchRungSample is one resolution rung the benchmark measured: the evidence
// behind a ceiling (e.g. why HDR floored to 480p).
type BenchRungSample = {
  ladder: string; // "software" | "software-hdr" | "hw"
  height: number;
  fps: number;
  required_fps: number;
  passing: boolean;
  parallel_encodes: number;
};

type EncodeCapability = {
  software_max_height: number;
  software_hdr_max_height?: number; // software ceiling for HDR->SDR (10-bit decode + tonemap)
  hw_max_height: number;
  hw_backend?: string;
  hw_codecs?: string[];       // codecs the GPU can ENCODE
  hw_decode_codecs?: string[]; // codecs the GPU can DECODE (may differ from encode)
  hw_device?: string;
  hw_10bit_transcode?: boolean;
  hw_tonemap?: boolean;
  hw_overlay?: boolean;
  hw_hdr10_encode?: boolean;
  hw_features?: string[]; // probed encoder options (low_power_h264, bframes, ...)
  gpu_class?: string; // "integrated" (iGPU/QSV) | "discrete" (NVENC) | "" (AMD/unknown)
  hw_mode?: string; // active policy: auto | off | nvenc | vaapi | qsv
  detected_backends?: string[]; // accelerators the host advertises
  is_wsl?: boolean; // WSL2: VAAPI/QSV (Mesa d3d12) unsupported, greyed out
  benchmarked_at: string; // RFC3339
  benchmark_duration_ms: number;
  hw_fallbacks?: HWFallback[];
  bench_samples?: BenchRungSample[]; // per-rung fps evidence behind the caps
  bench_contended?: boolean; // benchmarked while the host was busy: provisional, auto re-benched when idle
  bench_host_busy_percent?: number; // host-wide CPU busy share right before the bench (0-100)
};

const QK_CALIBRATION = ["admin", "transcode-calibration"] as const;

// gpuPipelineSummary lists what the calibrated GPU was probed to do, so the
// operator sees which pipelines it gets (and which it does not) at a glance.
export function gpuPipelineSummary(
  data: Pick<EncodeCapability, "hw_10bit_transcode" | "hw_tonemap" | "hw_overlay" | "hw_hdr10_encode" | "hw_features">,
  t: (key: string, fallback: string) => string,
): string[] {
  const yes = (v: boolean | undefined, label: string) => `${label}: ${v ? t("admin.calibration.yes", "yes") : t("admin.calibration.no", "no")}`;
  const out = [
    yes(data.hw_tonemap, t("admin.calibration.gpuTonemap", "HDR→SDR")),
    yes(data.hw_10bit_transcode, t("admin.calibration.gpu10bit", "10-bit")),
    yes(data.hw_hdr10_encode, t("admin.calibration.gpuHdr10", "HDR10 out (HEVC Main10)")),
    yes(data.hw_overlay, t("admin.calibration.gpuOverlay", "subtitle overlay")),
  ];
  const lowPower = (data.hw_features ?? [])
    .filter((f) => f.startsWith("low_power_"))
    .map((f) => f.slice("low_power_".length));
  out.push(
    lowPower.length > 0
      ? `${t("admin.calibration.gpuLowPower", "low power")}: ${lowPower.join(", ")}`
      : yes(false, t("admin.calibration.gpuLowPower", "low power")),
  );
  return out;
}
const SETTINGS_QK = ["/api/v1/admin/server-settings"] as const;

// EncodeCalibrationCard surfaces the host's measured encode ceiling and
// exposes the "Recalibrate" button that triggers a fresh benchmark.
// Read-only block above + action button below - kept small because the
// data is just a few lines (cap + last-bench timestamp).
export function EncodeCalibrationCard() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, isError } = useQuery<EncodeCapability>({
    queryKey: QK_CALIBRATION,
    queryFn: () => eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration", method: "GET" }),
  });

  // Server settings carry the AV1 output opt-in + height cap.
  const { data: settings } = useQuery<{ av1Enabled: { value: boolean }; av1MaxHeight: { value: number } }>({
    queryKey: SETTINGS_QK,
    queryFn: () => eveyFetch({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });
  const av1Enabled = settings?.av1Enabled?.value ?? false;
  const av1MaxHeight = settings?.av1MaxHeight?.value ?? 0;

  const setAV1 = useMutation({
    mutationFn: (patch: { av1Enabled?: boolean; av1MaxHeight?: number }) =>
      eveyFetch({ url: "/api/v1/admin/server-settings", method: "PUT", data: patch }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: SETTINGS_QK });
      toast.success(t("admin.calibration.av1Applied", "AV1 setting applied"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  // Local mirror of the active mode so the <select> stays responsive while the
  // detached re-bench runs server-side (the query refetches a few seconds later).
  const [hwMode, setHwMode] = useState<string>("auto");
  useEffect(() => {
    if (data?.hw_mode) setHwMode(data.hw_mode);
  }, [data?.hw_mode]);

  const setMode = useMutation({
    mutationFn: (mode: string) =>
      eveyFetch({ url: "/api/v1/admin/server-settings", method: "PUT", data: { hwMode: mode } }),
    onSuccess: (_resp, mode) => {
      setHwMode(mode);
      toast.success(t("admin.calibration.modeApplied", "Mode applied - recalibrating…"));
      // The mode change re-benches detached on the server (~20 s); refetch a few
      // times so the active backend + ceilings update once it lands.
      qc.invalidateQueries({ queryKey: QK_CALIBRATION });
      qc.invalidateQueries({ queryKey: SETTINGS_QK });
      [3000, 9000, 20000].forEach((ms) =>
        setTimeout(() => qc.invalidateQueries({ queryKey: QK_CALIBRATION }), ms),
      );
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const recalibrate = useMutation({
    mutationFn: () =>
      eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration/recalibrate", method: "POST" }),
    onSuccess: (fresh) => {
      qc.setQueryData(QK_CALIBRATION, fresh);
      toast.success(t("admin.calibration.successToast", "Recalibration complete"));
    },
    onError: (err: Error) => {
      // 409 → either transcodes are active (idle-gate: a contended bench would
      // pin the cap too low) or another recalibration is already running.
      // 500 / other → real failure. All surface as a toast; the cache stays
      // untouched so the previous reading still shows.
      if (/transcodes[_ ]active/i.test(err.message)) {
        toast.error(
          t(
            "admin.calibration.transcodesActiveToast",
            "Stop active playback before recalibrating - the benchmark needs an idle host to measure the true ceiling.",
          ),
        );
      } else if (/409|already in progress/i.test(err.message)) {
        toast.error(t("admin.calibration.busyToast", "Recalibration already in progress"));
      } else {
        toast.error(t("admin.calibration.errorToast", "Recalibration failed"));
      }
    },
  });

  if (isLoading) {
    return <div className="animate-shimmer h-32 rounded-[var(--radius-md)]" />;
  }

  if (isError || !data) {
    return (
      <EmptyState
        icon={<TriangleAlert size={32} />}
        title={t("common.errorBoundary.title")}
        description={t("common.errorBoundary.description")}
      />
    );
  }

  const fmtDate = (iso: string) => {
    try {
      return new Date(iso).toLocaleString(i18n.language);
    } catch {
      return iso;
    }
  };

  const fallbackReason = (cls: string): string => {
    switch (cls) {
      case "hw_unsupported_option":
        return t("admin.calibration.fallbackUnsupportedOption", "unsupported encoder option");
      case "hw_encoder_missing":
        return t("admin.calibration.fallbackEncoderMissing", "encoder unavailable");
      case "hw_init":
        return t("admin.calibration.fallbackInit", "GPU init failed");
      default:
        return cls;
    }
  };

  const fallbacks = data.hw_fallbacks ?? [];
  const detected = data.detected_backends ?? [];
  const isWsl = data.is_wsl ?? false;
  const hwEnabled = hwMode !== "off";
  // Forceable backends = detected ones, plus the current pin even if detection
  // now reports it absent (so a forced mode is never silently dropped). VAAPI/QSV
  // run through Mesa's d3d12 driver on WSL, which can't sustain our multi-variant
  // encode, so they're disabled there.
  const forceable = Array.from(
    new Set([...detected, ["nvenc", "vaapi", "qsv"].includes(hwMode) ? hwMode : ""].filter(Boolean)),
  );
  const wslBlocked = (b: string) => isWsl && (b === "vaapi" || b === "qsv");

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-2">
        {/* Master enable: off = software only, on = hardware (then pick a backend). */}
        <div className="flex items-center gap-3">
          <label className="text-sm font-medium text-[var(--text)]">
            {t("admin.calibration.hwEnableLabel", "Hardware acceleration")}
          </label>
          <Switch
            checked={hwEnabled}
            onChange={() => setMode.mutate(hwEnabled ? "off" : "auto")}
            label={t("admin.calibration.hwEnableLabel", "Hardware acceleration")}
            disabled={setMode.isPending}
            data-testid="hw-enable-toggle"
          />
          <span className="text-xs text-[var(--text-muted)]">
            {hwEnabled
              ? t("admin.calibration.hwEnabledHint", "Encoding on the GPU when possible")
              : t("admin.calibration.hwDisabledHint", "Software only (CPU)")}
          </span>
        </div>

        {hwEnabled && (
          <div className="flex flex-col gap-1">
            <label htmlFor="hw-mode-select" className="text-xs uppercase tracking-wider text-[var(--text-muted)]">
              {t("admin.calibration.hwBackendLabel", "Backend")}
            </label>
            <select
              id="hw-mode-select"
              data-testid="hw-mode-select"
              value={hwMode}
              disabled={setMode.isPending}
              onChange={(e) => setMode.mutate(e.target.value)}
              className="w-full max-w-xs rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] disabled:opacity-60"
            >
              <option value="auto">{t("admin.calibration.hwModeAuto", "Auto (best available)")}</option>
              {forceable.map((b) => (
                <option key={b} value={b} disabled={wslBlocked(b)}>
                  {b.toUpperCase()}
                  {wslBlocked(b) ? ` - ${t("admin.calibration.wslUnsupported", "not supported on WSL")}` : ""}
                </option>
              ))}
            </select>
            <p className="text-xs text-[var(--text-muted)]">
              {detected.length > 0
                ? t("admin.calibration.detected", "Detected: {{list}}", { list: detected.join(", ") })
                : t("admin.calibration.detectedNone", "No GPU accelerator detected")}
            </p>
            {isWsl && (
              <p className="text-xs text-[var(--color-warning)]">
                {t(
                  "admin.calibration.wslVaapiNote",
                  "VAAPI/QSV go through Mesa's d3d12 driver on WSL and can't sustain transcoding here - use NVENC, or validate VAAPI on a native Linux host.",
                )}
              </p>
            )}

            {/* AV1 output - hardware-only, opt-in. Shown only with HW on (this
                block) because software AV1 is never used for streaming. */}
            <div className="mt-2 flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">
                {t("admin.calibration.av1Label", "AV1 output")}
              </label>
              <Switch
                checked={av1Enabled}
                onChange={() => setAV1.mutate({ av1Enabled: !av1Enabled })}
                label={t("admin.calibration.av1Label", "AV1 output")}
                disabled={setAV1.isPending}
                data-testid="av1-enable-toggle"
              />
            </div>
            <p className="text-xs text-[var(--text-muted)]">
              {(data.hw_codecs ?? []).includes("av1")
                ? t("admin.calibration.av1Hint", "Encode to AV1 when a client supports it - better compression. Hardware-only; software AV1 is never used for streaming.")
                : t("admin.calibration.av1NoHw", "No AV1 hardware encoder detected - turning this on has no effect (streams stay H.264).")}
            </p>

            {av1Enabled && (
              <div className="flex flex-col gap-1">
                <label htmlFor="av1-max-height" className="text-xs uppercase tracking-wider text-[var(--text-muted)]">
                  {t("admin.calibration.av1MaxLabel", "AV1 max quality")}
                </label>
                <select
                  id="av1-max-height"
                  data-testid="av1-max-height-select"
                  value={String(av1MaxHeight)}
                  disabled={setAV1.isPending}
                  onChange={(e) => setAV1.mutate({ av1MaxHeight: parseInt(e.target.value, 10) })}
                  className="w-full max-w-xs rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm text-[var(--text)] disabled:opacity-60"
                >
                  <option value="0">{t("admin.calibration.av1MaxAuto", "Auto (up to source)")}</option>
                  <option value="720">720p</option>
                  <option value="1080">1080p</option>
                  <option value="1440">1440p</option>
                  <option value="2160">2160p</option>
                </select>
              </div>
            )}
          </div>
        )}
      </div>

      <div className="grid grid-cols-2 gap-3 text-sm">
        <Row label={t("admin.calibration.softwareMax", "Software encode cap")}>
          <strong className="text-[var(--text)]">{data.software_max_height}p</strong>
          <span className="text-[var(--text-muted)]"> ({t("admin.calibration.libx264", "libx264 veryfast")})</span>
        </Row>
        {data.software_hdr_max_height ? (
          <Row label={t("admin.calibration.softwareHdrMax", "Software HDR cap")}>
            <strong className="text-[var(--text)]">{data.software_hdr_max_height}p</strong>
            <span className="text-[var(--text-muted)]"> ({t("admin.calibration.hdrTonemap", "HDR→SDR tonemap")})</span>
          </Row>
        ) : null}
        <Row label={t("admin.calibration.hwMax", "Hardware encode cap")}>
          {data.hw_max_height > 0 ? (
            <>
              <strong className="text-[var(--text)]">{data.hw_max_height}p</strong>
              <span className="text-[var(--text-muted)]"> ({data.hw_backend}{
                data.gpu_class === "integrated" ? `, ${t("admin.calibration.gpuIntegrated", "iGPU")}`
                  : data.gpu_class === "discrete" ? `, ${t("admin.calibration.gpuDiscrete", "dedicated GPU")}`
                    : ""})</span>
              {(data.hw_codecs?.length || data.hw_device) && (
                <span className="mt-0.5 block text-xs text-[var(--text-muted)]">
                  {[data.hw_codecs?.join(", "), data.hw_device]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
              )}
              <span className="mt-0.5 block text-xs text-[var(--text-muted)]" data-testid="gpu-pipeline">
                {gpuPipelineSummary(data, t).join(" · ")}
              </span>
            </>
          ) : (
            <span className="text-[var(--text-muted)]">{t("admin.calibration.hwNone", "no GPU accelerator detected")}</span>
          )}
        </Row>
        <Row label={t("admin.calibration.lastRun", "Last calibrated")}>
          <span className="text-[var(--text)]">{fmtDate(data.benchmarked_at)}</span>
          <span className="text-[var(--text-muted)]"> ({(data.benchmark_duration_ms / 1000).toFixed(1)}s)</span>
        </Row>
      </div>

      {/* A bench run on an already-busy host measured the neighbours, not the
          box: say so next to the ceilings it produced, or a shared-box floor
          reads as this host's ceiling. The server re-benches by itself. */}
      {data.bench_contended && (
        <p className="mt-2 text-xs font-medium text-[var(--color-warning)]" data-testid="calibration-contended">
          {t(
            "admin.calibration.contendedLine",
            "Provisional: the server was already busy when it measured itself (CPU at {{percent}}%), so these ceilings are underestimated - they describe a shared machine, not this one. It will measure again on its own once idle.",
            { percent: data.bench_host_busy_percent ?? 0 },
          )}
        </p>
      )}

      {data.bench_samples && data.bench_samples.length > 0 ? (
        <details className="mt-3 text-xs">
          <summary className="cursor-pointer text-[var(--text-muted)]">
            {t("admin.calibration.samplesTitle", "Benchmark measurements (fps per rung)")}
          </summary>
          <table className="mt-2 w-full border-collapse">
            <thead>
              <tr className="text-left text-[var(--text-muted)]">
                <th className="py-1 pr-3 font-medium">{t("admin.calibration.samplesLadder", "Path")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.calibration.samplesHeight", "Rung")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.calibration.samplesFps", "fps")}</th>
                <th className="py-1 pr-3 font-medium">{t("admin.calibration.samplesNeeded", "needed")}</th>
                <th className="py-1 font-medium">{t("admin.calibration.samplesResult", "result")}</th>
              </tr>
            </thead>
            <tbody>
              {data.bench_samples.map((s, i) => (
                <tr key={`${s.ladder}-${s.height}-${i}`} className="text-[var(--text)]">
                  <td className="py-0.5 pr-3 text-[var(--text-muted)]">{s.ladder}</td>
                  <td className="py-0.5 pr-3">{s.height}p</td>
                  <td className="py-0.5 pr-3">{s.fps.toFixed(1)}</td>
                  <td className="py-0.5 pr-3 text-[var(--text-muted)]">{s.required_fps.toFixed(1)}</td>
                  <td className={`py-0.5 ${s.passing ? "text-[var(--success,#16a34a)]" : "text-[var(--danger,#dc2626)]"}`}>
                    {s.passing
                      ? t("admin.calibration.samplesPass", "pass")
                      : t("admin.calibration.samplesFail", "fail")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      ) : null}

      {(data.hw_codecs?.length || data.hw_decode_codecs?.length) ? (
        <CodecSupportBlock
          encodeCodecs={data.hw_codecs ?? []}
          decodeCodecs={data.hw_decode_codecs ?? []}
        />
      ) : null}

      {fallbacks.length > 0 && (
        <div className="rounded-[var(--radius-md)] border border-[color-mix(in_srgb,var(--color-warning)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_5%,transparent)] p-3">
          <p className="text-sm font-medium text-[var(--color-warning)]">
            {t("admin.calibration.hwFallbacks", "Hardware fallbacks")}
          </p>
          <p className="mt-0.5 text-xs text-[var(--text-muted)]">
            {t(
              "admin.calibration.hwFallbacksDesc",
              "Recent hardware encodes that fell back automatically - the host keeps playing via software while the cause is recorded. Recalibrate once the hardware is fixed.",
            )}
          </p>
          <ul className="mt-2 space-y-1 text-xs">
            {fallbacks
              .slice()
              .reverse()
              .map((f, i) => (
                <li key={i} className="text-[var(--text)]">
                  <code className="text-[var(--text)]">{f.codec || "h264"}</code>
                  {" → software · "}
                  {fallbackReason(f.class)}
                  {" · "}
                  <span className="text-[var(--text-muted)]">{fmtDate(f.at)}</span>
                </li>
              ))}
          </ul>
        </div>
      )}

      <div className="flex items-center gap-3">
        <Button
          variant="primary"
          onClick={() => recalibrate.mutate()}
          disabled={recalibrate.isPending}
          data-testid="recalibrate-btn"
        >
          {recalibrate.isPending
            ? t("admin.calibration.recalibrating", "Recalibrating…")
            : t("admin.calibration.recalibrate", "Recalibrate")}
        </Button>
        <p className="text-xs text-[var(--text-muted)]">
          {t(
            "admin.calibration.recalibrateHint",
            "Re-run the benchmark after enabling hardware acceleration, swapping the CPU, or migrating the container to a different host.",
          )}
        </p>
      </div>
    </div>
  );
}

function CodecSupportBlock({
  encodeCodecs,
  decodeCodecs,
}: {
  encodeCodecs: string[];
  decodeCodecs: string[];
}) {
  const { t } = useTranslation();
  // Codecs the GPU can encode but NOT decode (must fall back to software decode).
  const softwareDecodeOnly = encodeCodecs.filter((c) => !decodeCodecs.includes(c));

  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] p-3 text-xs flex flex-col gap-1.5">
      <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
        {t("admin.calibration.codecSupportTitle", "Codec support")}
      </p>
      {encodeCodecs.length > 0 && (
        <p className="text-[var(--text)]">
          <span className="text-[var(--text-muted)]">{t("admin.calibration.encodeCodecs", "Encode:")}</span>{" "}
          {encodeCodecs.map((c) => c.toUpperCase()).join(", ")}
        </p>
      )}
      {decodeCodecs.length > 0 && (
        <p className="text-[var(--text)]">
          <span className="text-[var(--text-muted)]">{t("admin.calibration.decodeCodecs", "Decode:")}</span>{" "}
          {decodeCodecs.map((c) => c.toUpperCase()).join(", ")}
        </p>
      )}
      {softwareDecodeOnly.length > 0 && (
        <p className="text-[var(--color-warning)]">
          {t("admin.calibration.codecSoftwareDecodeNote", "{{codecs}} - decoded in software (GPU encode only)", {
            codecs: softwareDecodeOnly.map((c) => c.toUpperCase()).join(", "),
          })}
        </p>
      )}
    </div>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col">
      <span className="text-xs uppercase tracking-wider text-[var(--text-muted)]">{label}</span>
      <div className="mt-0.5">{children}</div>
    </div>
  );
}
