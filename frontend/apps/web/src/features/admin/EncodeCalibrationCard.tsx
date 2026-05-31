import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";

// EncodeCapability is the JSON shape served by the backend at
// /api/v1/admin/transcode/calibration. Mirrors the Go domain struct.
type HWFallback = { class: string; codec: string; at: string };

type EncodeCapability = {
  software_max_height: number;
  hw_max_height: number;
  hw_backend?: string;
  hw_codecs?: string[];
  hw_device?: string;
  benchmarked_at: string; // RFC3339
  benchmark_duration_ms: number;
  cpu_signature: string;
  hw_fallbacks?: HWFallback[];
};

const QK_CALIBRATION = ["admin", "transcode-calibration"] as const;

// EncodeCalibrationCard surfaces the host's measured encode ceiling and
// exposes the "Recalibrate" button that triggers a fresh benchmark.
// Read-only block above + action button below — kept small because the
// data is just a few lines (cap + last-bench timestamp).
export function EncodeCalibrationCard() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading } = useQuery<EncodeCapability>({
    queryKey: QK_CALIBRATION,
    queryFn: () => eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration", method: "GET" }),
  });

  const recalibrate = useMutation({
    mutationFn: () =>
      eveyFetch<EncodeCapability>({ url: "/api/v1/admin/transcode/calibration/recalibrate", method: "POST" }),
    onSuccess: (fresh) => {
      qc.setQueryData(QK_CALIBRATION, fresh);
      toast.success(t("admin.calibration.successToast", "Recalibration complete"));
    },
    onError: (err: Error) => {
      // 409 → another recalibration already running (server-side lock).
      // 500 / other → real failure. Both surface to the admin as a
      // toast; the cache stays untouched so the previous reading still
      // shows.
      if (/409|already in progress/i.test(err.message)) {
        toast.error(t("admin.calibration.busyToast", "Recalibration already in progress"));
      } else {
        toast.error(t("admin.calibration.errorToast", "Recalibration failed"));
      }
    },
  });

  if (isLoading || !data) {
    return <div className="animate-shimmer h-32 rounded-[var(--radius-md)]" />;
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

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 text-sm">
        <Row label={t("admin.calibration.softwareMax", "Software encode cap")}>
          <strong className="text-[var(--text)]">{data.software_max_height}p</strong>
          <span className="text-[var(--text-muted)]"> ({t("admin.calibration.libx264", "libx264 ultrafast")})</span>
        </Row>
        <Row label={t("admin.calibration.hwMax", "Hardware encode cap")}>
          {data.hw_max_height > 0 ? (
            <>
              <strong className="text-[var(--text)]">{data.hw_max_height}p</strong>
              <span className="text-[var(--text-muted)]"> ({data.hw_backend})</span>
              {(data.hw_codecs?.length || data.hw_device) && (
                <span className="mt-0.5 block text-xs text-[var(--text-muted)]">
                  {[data.hw_codecs?.join(", "), data.hw_device]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
              )}
            </>
          ) : (
            <span className="text-[var(--text-muted)]">{t("admin.calibration.hwNone", "no GPU accelerator detected")}</span>
          )}
        </Row>
        <Row label={t("admin.calibration.lastRun", "Last calibrated")}>
          <span className="text-[var(--text)]">{fmtDate(data.benchmarked_at)}</span>
          <span className="text-[var(--text-muted)]"> ({(data.benchmark_duration_ms / 1000).toFixed(1)}s)</span>
        </Row>
        <Row label={t("admin.calibration.cpuSig", "CPU signature")}>
          <code className="text-xs text-[var(--text-muted)] truncate">{data.cpu_signature.slice(0, 12)}…</code>
        </Row>
      </div>

      {fallbacks.length > 0 && (
        <div className="rounded-[var(--radius-md)] border border-amber-500/40 bg-amber-500/5 p-3">
          <p className="text-sm font-medium text-amber-600">
            {t("admin.calibration.hwFallbacks", "Hardware fallbacks")}
          </p>
          <p className="mt-0.5 text-xs text-[var(--text-muted)]">
            {t(
              "admin.calibration.hwFallbacksDesc",
              "Recent hardware encodes that fell back automatically — the host keeps playing via software while the cause is recorded. Recalibrate once the hardware is fixed.",
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

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col">
      <span className="text-xs uppercase tracking-wider text-[var(--text-muted)]">{label}</span>
      <div className="mt-0.5">{children}</div>
    </div>
  );
}
