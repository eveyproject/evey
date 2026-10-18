import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Shield, AlertTriangle, XCircle, Clock, ChevronDown, ChevronUp, CheckCircle, ShieldOff } from "lucide-react";
import { eveyFetch } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { ICON } from "@/constants/iconSizes";

interface SecurityFinding {
  category: string;
  severity: string;
  file: string;
  line: number;
  description?: string;
}

interface SignatureStatus {
  verified: boolean;
  verifiedBy?: string;
  error?: string;
}

interface PluginLastError {
  ts: string;
  message: string;
}

interface PluginSecuritySnapshot {
  id: string;
  name: string;
  version: string;
  sandboxMode: string;
  permissions: string[];
  findings: SecurityFinding[];
  lastError?: PluginLastError;
  signature?: SignatureStatus;
  sourceSHA256: string;
  installedAt: string;
  lastStartedAt?: string;
  sizeBytes: number;
}

interface SandboxStatus {
  configured: string;
  effective: string;
  landlockKernel: boolean;
  launcherPresent: boolean;
  bubblewrapUsable: boolean;
}

function sandboxBadgeVariant(mode: string): "success" | "warning" | "default" {
  if (mode === "strict" || mode === "bubblewrap" || mode === "landlock") return "success";
  if (mode === "basic") return "warning";
  return "default";
}

// Host-level sandbox summary: which isolation is actually in force (the runner
// resolves "auto"), with the probe results explaining a degradation to basic.
// Shown even with zero plugins installed - that's exactly when an admin should
// learn their host can't isolate plugins.
function SandboxStatusCard({ sandbox }: { sandbox: SandboxStatus }) {
  const { t } = useTranslation();
  const degraded = sandbox.effective === "basic";
  const probes: Array<{ key: string; ok: boolean; label: string }> = [
    { key: "landlock", ok: sandbox.landlockKernel, label: t("admin.plugins.security.probeLandlock") },
    { key: "launcher", ok: sandbox.launcherPresent, label: t("admin.plugins.security.probeLauncher") },
    { key: "bwrap", ok: sandbox.bubblewrapUsable, label: t("admin.plugins.security.probeBubblewrap") },
  ];

  return (
    <div
      className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] px-4 py-3 flex flex-col gap-2"
      data-testid="sandbox-status-card"
    >
      <div className="flex items-center gap-2 flex-wrap text-sm">
        <span className="text-[var(--text-muted)]">{t("admin.plugins.security.sandboxHost")}</span>
        <Badge variant={sandboxBadgeVariant(sandbox.effective)}>{sandbox.effective}</Badge>
        {sandbox.configured !== sandbox.effective && (
          <span className="text-xs text-[var(--text-muted)]">
            {t("admin.plugins.security.sandboxResolvedFrom", { mode: sandbox.configured })}
          </span>
        )}
      </div>
      {degraded && (
        <div
          className="text-xs p-3 rounded-[var(--radius-sm)] bg-[var(--badge-warning-bg,#713f12)]/40 border border-[var(--badge-warning-text,#fde047)]/30 flex flex-col gap-1.5"
          data-testid="sandbox-degraded-warning"
        >
          <p className="text-[var(--badge-warning-text,#fde047)] font-medium">
            {t("admin.plugins.security.sandboxBasicWarning")}
          </p>
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-[var(--text-muted)]">
            {probes.map((p) => (
              <span key={p.key}>
                <span className={p.ok ? "text-[var(--badge-success-text,#6ee7b7)]" : "text-[var(--badge-error-text,#fca5a5)]"}>
                  {p.ok ? "✓" : "✗"}
                </span>{" "}
                {p.label}
              </span>
            ))}
          </div>
          <p className="text-[var(--text-muted)]">{t("admin.plugins.security.sandboxBasicHint")}</p>
        </div>
      )}
    </div>
  );
}

function countBySeverity(findings: SecurityFinding[]) {
  return findings.reduce(
    (acc, f) => {
      if (f.severity === "critical") acc.critical++;
      else if (f.severity === "warning") acc.warning++;
      else acc.info++;
      return acc;
    },
    { critical: 0, warning: 0, info: 0 },
  );
}

function SnapshotRow({ snap }: { snap: PluginSecuritySnapshot }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const counts = countBySeverity(snap.findings ?? []);
  const hasFindings = (snap.findings ?? []).length > 0;
  const hasError = !!snap.lastError;

  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] overflow-hidden">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="w-full flex items-center justify-between gap-3 px-4 py-3 text-left hover:bg-[var(--surface-raised)] transition-colors-fast cursor-pointer"
      >
        <div className="flex flex-col gap-0.5 min-w-0">
          <span className="font-semibold text-[var(--text)] truncate">{snap.name}</span>
          <span className="text-xs text-[var(--text-muted)]">
            v{snap.version}
            {snap.sourceSHA256 && (
              <span className="ml-2 font-mono opacity-60">{snap.sourceSHA256.slice(0, 8)}</span>
            )}
          </span>
        </div>
        <div className="flex items-center gap-2 flex-wrap shrink-0">
          <Badge variant={sandboxBadgeVariant(snap.sandboxMode)}>
            {snap.sandboxMode}
          </Badge>
          {snap.permissions?.includes("subprocess") && (
            <Badge variant="warning">{t("admin.plugin.subprocess")}</Badge>
          )}
          {counts.critical > 0 ? (
            <Badge variant="error">
              {counts.critical} {t("admin.plugins.security.critical")}
            </Badge>
          ) : counts.warning > 0 ? (
            <Badge variant="warning">
              {counts.warning} {t("admin.plugins.security.warnings")}
            </Badge>
          ) : (
            <Badge variant="success">{t("admin.plugins.security.clean")}</Badge>
          )}
          {snap.signature && (
            snap.signature.verified ? (
              <Badge variant="success">
                <CheckCircle size={ICON.xs} className="mr-1" />
                {t("admin.plugins.security.signed")}
              </Badge>
            ) : (
              <Badge variant="warning">
                <ShieldOff size={ICON.xs} className="mr-1" />
                {t("admin.plugins.security.unsigned")}
              </Badge>
            )
          )}
          {hasError && (
            <Badge variant="error">{t("admin.plugins.security.errorBadge")}</Badge>
          )}
          {(hasFindings || hasError) ? (
            expanded ? <ChevronUp size={ICON.sm} className="text-[var(--text-muted)]" /> : <ChevronDown size={ICON.sm} className="text-[var(--text-muted)]" />
          ) : null}
        </div>
      </button>

      {expanded && (hasFindings || hasError) && (
        <div className="px-4 pb-4 flex flex-col gap-3 border-t border-[var(--border)]">
          {hasFindings && (
            <div className="flex flex-col gap-1.5 pt-3">
              <span className="text-xs font-semibold text-[var(--text-muted)]">
                {t("admin.plugins.security.findings")}
              </span>
              {snap.findings.map((f, i) => (
                <div key={i} className="flex items-start gap-2 text-xs">
                  {f.severity === "critical" ? (
                    <XCircle size={ICON.xs} className="text-[var(--badge-error-text,#fca5a5)] shrink-0 mt-0.5" />
                  ) : (
                    <AlertTriangle size={ICON.xs} className="text-[var(--badge-warning-text,#fde047)] shrink-0 mt-0.5" />
                  )}
                  <span
                    className={
                      f.severity === "critical"
                        ? "text-[var(--badge-error-text,#fca5a5)] font-semibold"
                        : "text-[var(--badge-warning-text,#fde047)] font-semibold"
                    }
                  >
                    {f.severity.toUpperCase()}
                  </span>
                  <span className="text-[var(--text)] font-medium">{f.category}</span>
                  {f.description && (
                    <span className="text-[var(--text-muted)] truncate">{f.description}</span>
                  )}
                  {f.file && (
                    <span className="font-mono text-[var(--text-muted)] ml-auto shrink-0">
                      {f.file}:{f.line}
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}

          {hasError && snap.lastError && (
            <div className="flex items-start gap-2 text-xs p-3 rounded-[var(--radius-sm)] bg-[var(--badge-error-bg,#7f1d1d)] border border-[var(--badge-error-text,#fca5a5)]/30">
              <Clock size={ICON.xs} className="text-[var(--badge-error-text,#fca5a5)] shrink-0 mt-0.5" />
              <div className="min-w-0">
                <span className="font-semibold text-[var(--badge-error-text,#fca5a5)]">
                  {t("admin.plugins.security.lastError")}
                </span>
                <span className="ml-1 text-[var(--text-muted)]">
                  {new Date(snap.lastError.ts).toLocaleString()}
                </span>
                <p className="text-[var(--text-muted)] mt-0.5 break-all">
                  {snap.lastError.message}
                </p>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function PluginSecurityDashboard() {
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["/api/v1/admin/plugins/security-dashboard"],
    queryFn: () =>
      eveyFetch<{ plugins: PluginSecuritySnapshot[]; sandbox?: SandboxStatus }>({
        url: "/api/v1/admin/plugins/security-dashboard",
        method: "GET",
      }),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });

  const snapshots = data?.plugins ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <Shield size={ICON.md} className="text-[var(--text-muted)]" />
        <h3 className="text-base font-semibold text-[var(--text)]">
          {t("admin.plugins.security.title")}
        </h3>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="sm" />
        </div>
      ) : isError && !data ? (
        // A failed read is not "no data": the degraded-sandbox warning would
        // vanish behind an empty-looking dashboard.
        <FetchErrorState error={error} onRetry={() => refetch()} />
      ) : (
        <>
          {data?.sandbox && <SandboxStatusCard sandbox={data.sandbox} />}
          {snapshots.length === 0 ? (
            <p className="text-sm text-[var(--text-muted)] py-2">
              {t("admin.plugins.security.noData")}
            </p>
          ) : (
            <div className="flex flex-col gap-2">
              {snapshots.map((snap) => (
                <SnapshotRow key={snap.id} snap={snap} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
