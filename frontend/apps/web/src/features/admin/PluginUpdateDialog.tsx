import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { type PluginSignature, shortFingerprint, signatureKind } from "./pluginSignature";

// SecurityFinding mirrors the proto message SecurityFinding
interface SecurityFinding {
  category?: string;
  severity?: string;
  file?: string;
  line?: number;
  description?: string;
}

interface PreviewResponse {
  currentVersion?: string;
  candidateVersion?: string;
  permissionsAdded?: string[];
  permissionsRemoved?: string[];
  capabilitiesAdded?: string[];
  capabilitiesRemoved?: string[];
  slotsAdded?: string[];
  slotsRemoved?: string[];
  externalFetchAdded?: string[];
  externalFetchRemoved?: string[];
  newFindings?: SecurityFinding[];
  signatureStatus?: string;
  signature?: PluginSignature;
  requiresAck?: boolean;
  sourceHash?: string;
}

type Phase = "previewing" | "reviewing" | "applying" | "error";

type TabKey = "permissions" | "slots" | "externalFetch" | "findings" | "signature";

function severityColor(severity?: string): string {
  switch ((severity ?? "").toLowerCase()) {
    case "critical": return "var(--badge-error-text)";
    case "warning":  return "var(--badge-warning-text)";
    default:         return "var(--text-muted)";
  }
}

function DiffList({
  added,
  removed,
  emptyLabel,
}: {
  added?: string[];
  removed?: string[];
  emptyLabel: string;
}) {
  const hasContent = (added?.length ?? 0) > 0 || (removed?.length ?? 0) > 0;
  if (!hasContent) {
    return <p className="text-xs text-[var(--text-muted)] py-2">{emptyLabel}</p>;
  }
  return (
    <ul className="flex flex-col gap-1 text-xs mt-2">
      {(added ?? []).map((item) => (
        <li key={`add-${item}`} className="flex items-center gap-2">
          <span
            className="font-semibold"
            style={{ color: "var(--badge-error-text)" }}
            data-testid="permission-added"
          >
            +
          </span>
          <span style={{ color: "var(--badge-error-text)" }}>{item}</span>
        </li>
      ))}
      {(removed ?? []).map((item) => (
        <li key={`rem-${item}`} className="flex items-center gap-2">
          <span className="font-semibold text-[var(--text-muted)]">-</span>
          <span className="text-[var(--text-muted)]">{item}</span>
        </li>
      ))}
    </ul>
  );
}

function SignatureTab({ preview }: { preview: PreviewResponse }) {
  const { t } = useTranslation();
  const sig = preview.signature;
  const kind = signatureKind(sig);
  const continuityKey: Record<string, string> = {
    same: "continuitySame",
    changed: "continuityChanged",
    lost: "continuityLost",
  };
  return (
    <div className="py-2 flex flex-col gap-2 text-xs">
      <div>
        {!sig && (preview.signatureStatus === "signed" || preview.signatureStatus === "SIGNED") ? (
          <Badge variant="success">{t("admin.plugins.security.signed")}</Badge>
        ) : kind === "trusted" ? (
          <Badge variant="success">{t("admin.plugin.signature.signedBy", { name: sig?.publisher })}</Badge>
        ) : kind === "untrusted" ? (
          <Badge variant="warning">{t("admin.plugin.signature.signedUnknown", { fp: shortFingerprint(sig?.keyFingerprint) })}</Badge>
        ) : kind === "invalid" || preview.signatureStatus === "invalid" ? (
          <Badge variant="error">{t("admin.plugin.updateDialog.signatureInvalid")}</Badge>
        ) : (
          <Badge variant="warning">{t("admin.plugins.security.unsigned")}</Badge>
        )}
      </div>
      {sig?.publisher && (
        <p className="text-[var(--text-muted)]">
          {t("admin.plugin.signature.publisher")}: <span className="text-[var(--text)]">{sig.publisher}</span>
        </p>
      )}
      {sig?.keyFingerprint && (
        <p className="text-[var(--text-muted)]">
          {t("admin.plugin.signature.fingerprint")}: <span className="font-mono break-all text-[var(--text)]">{sig.keyFingerprint}</span>
        </p>
      )}
      <p className="text-[var(--text-muted)]">
        {t("admin.plugin.signature.continuity")}:{" "}
        <span className="text-[var(--text)]" data-testid="signature-continuity">
          {t(`admin.plugin.signature.${continuityKey[sig?.continuity ?? ""] ?? "continuityNone"}`)}
        </span>
      </p>
    </div>
  );
}

export type Props = {
  pluginId: string;
  pluginName?: string;
  source: string;
  onClose: () => void;
  onApplied: () => void;
};

export function PluginUpdateDialog({ pluginId, pluginName, source, onClose, onApplied }: Props) {
  const { t } = useTranslation();
  const [phase, setPhase] = useState<Phase>("previewing");
  const [preview, setPreview] = useState<PreviewResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabKey>("permissions");
  const [ackChecked, setAckChecked] = useState(false);
  const [publisherAck, setPublisherAck] = useState(false);

  const tabLabels: Record<TabKey, string> = {
    permissions: t("admin.plugins.permissions"),
    slots: t("admin.plugin.updateDialog.tabSlots"),
    externalFetch: t("admin.plugin.updateDialog.tabExternalFetch"),
    findings: t("admin.plugin.updateDialog.tabFindings"),
    signature: t("admin.plugin.updateDialog.tabSignature"),
  };

  useEffect(() => {
    let cancelled = false;
    async function doPreview() {
      setPhase("previewing");
      setError(null);
      try {
        const data = await eveyFetch<PreviewResponse>({
          url: `/api/v1/admin/plugins/${pluginId}/preview-update`,
          method: "POST",
          data: { source },
        });
        if (!cancelled) {
          setPreview(data);
          setPhase("reviewing");
        }
      } catch (err: unknown) {
        if (!cancelled) {
          const msg = err instanceof Error ? err.message : t("admin.plugin.updateDialog.previewError");
          setError(msg);
          setPhase("error");
        }
      }
    }
    void doPreview();
    return () => { cancelled = true; };
  }, [pluginId, source, t]);

  async function handleApply() {
    if (!preview?.sourceHash) return;
    setPhase("applying");
    setError(null);
    try {
      await eveyFetch<unknown>({
        url: `/api/v1/admin/plugins/${pluginId}/apply-update`,
        method: "POST",
        data: { source, acknowledged_hash: preview.sourceHash, accept_new_publisher: publisherAck },
      });
      onApplied();
      onClose();
    } catch (err: unknown) {
      // HTTP 409 or gRPC Aborted → candidate changed between preview and apply
      const isAborted =
        (err instanceof Error && (err.message.includes("409") || err.message.toLowerCase().includes("aborted"))) ||
        (typeof err === "object" && err !== null && "status" in err && (err as { status: number }).status === 409);
      const msg = isAborted
        ? t("admin.plugin.updateDialog.hashMismatch")
        : err instanceof Error
          ? err.message
          : t("admin.plugin.updateDialog.applyError");
      setError(msg);
      setPhase("reviewing");
    }
  }

  const title = preview
    ? t("admin.plugin.updateDialog.titleWithVersions", {
        name: pluginName ?? pluginId,
        from: preview.currentVersion ?? "?",
        to: preview.candidateVersion ?? "?",
      })
    : t("admin.plugin.updateDialog.title", { name: pluginName ?? pluginId });

  // The continuity block is the publisher-changed / signature-lost verdict, separate from the permission ack.
  const publisherBlock =
    preview?.signatureStatus === "publisher-changed" || preview?.signatureStatus === "signature-lost";
  const widens = [preview?.permissionsAdded, preview?.capabilitiesAdded, preview?.slotsAdded, preview?.externalFetchAdded]
    .some((l) => (l?.length ?? 0) > 0) || (preview?.newFindings?.length ?? 0) > 0;
  const needsPermAck = !!preview?.requiresAck && (!publisherBlock || widens);

  const applyDisabled =
    phase === "previewing" ||
    phase === "applying" ||
    phase === "error" && preview === null ||
    (needsPermAck && !ackChecked) ||
    (publisherBlock && !publisherAck);

  return (
    <Modal open onClose={onClose} title={title} size="xl">
      {phase === "previewing" && (
        <div className="flex flex-col items-center gap-3 py-8">
          <Spinner size="md" />
          <p className="text-sm text-[var(--text-muted)]">{t("admin.plugin.updateDialog.analyzing")}</p>
        </div>
      )}

      {(phase === "reviewing" || phase === "applying" || (phase === "error" && preview !== null)) && preview && (
        <div className="flex flex-col gap-4">
          {/* Security banner */}
          {needsPermAck ? (
            <div
              className="rounded border px-3 py-2 text-sm font-semibold"
              style={{
                borderColor: "var(--color-danger)",
                background: "var(--badge-error-bg, rgba(127,29,29,0.15))",
                color: "var(--badge-error-text)",
              }}
              data-testid="banner-requires-ack"
            >
              {t("admin.plugin.updateDialog.bannerRequiresAck")}
            </div>
          ) : (
            <div
              className="rounded border px-3 py-2 text-sm font-semibold"
              style={{
                borderColor: "var(--color-success)",
                background: "var(--badge-success-bg, rgba(20,83,45,0.15))",
                color: "var(--badge-success-text)",
              }}
              data-testid="banner-safe"
            >
              {t("admin.plugin.updateDialog.bannerSafe")}
            </div>
          )}

          {publisherBlock && (
            <div
              className="rounded border px-3 py-2 text-sm font-semibold"
              style={{
                borderColor: "var(--color-danger)",
                background: "var(--badge-error-bg, rgba(127,29,29,0.15))",
                color: "var(--badge-error-text)",
              }}
              data-testid="banner-publisher"
            >
              {preview.signatureStatus === "signature-lost"
                ? t("admin.plugin.signature.bannerLost")
                : t("admin.plugin.signature.bannerChanged")}
            </div>
          )}

          {/* Tabs */}
          <div className="flex gap-1 border-b border-[var(--border)]">
            {(Object.keys(tabLabels) as TabKey[]).map((tab) => (
              <button
                key={tab}
                type="button"
                className={`px-3 py-1.5 text-xs font-medium rounded-t transition-colors ${
                  activeTab === tab
                    ? "bg-[var(--surface-raised)] text-[var(--text)]"
                    : "text-[var(--text-muted)] hover:text-[var(--text)]"
                }`}
                onClick={() => setActiveTab(tab)}
              >
                {tabLabels[tab]}
              </button>
            ))}
          </div>

          {/* Tab content */}
          <div className="min-h-[100px]">
            {activeTab === "permissions" && (
              <DiffList
                added={preview.permissionsAdded}
                removed={preview.permissionsRemoved}
                emptyLabel={t("admin.plugin.updateDialog.emptyPermissions")}
              />
            )}
            {activeTab === "slots" && (
              <DiffList
                added={preview.slotsAdded}
                removed={preview.slotsRemoved}
                emptyLabel={t("admin.plugin.updateDialog.emptySlots")}
              />
            )}
            {activeTab === "externalFetch" && (
              <DiffList
                added={preview.externalFetchAdded}
                removed={preview.externalFetchRemoved}
                emptyLabel={t("admin.plugin.updateDialog.emptyExternalFetch")}
              />
            )}
            {activeTab === "findings" && (
              <>
                {(preview.newFindings?.length ?? 0) === 0 ? (
                  <p className="text-xs text-[var(--text-muted)] py-2">{t("admin.plugin.updateDialog.noFindings")}</p>
                ) : (
                  <ul className="flex flex-col gap-2 text-xs mt-2">
                    {preview.newFindings!.map((f, i) => (
                      <li key={i} className="flex flex-col gap-0.5">
                        <div className="flex items-center gap-2">
                          <span
                            className="font-semibold uppercase"
                            style={{ color: severityColor(f.severity) }}
                          >
                            {f.severity ?? "INFO"}
                          </span>
                          <span className="text-[var(--text-muted)]">{f.category}</span>
                        </div>
                        <span className="text-[var(--text)]">{f.description}</span>
                        {f.file && (
                          <span className="font-mono text-[var(--text-muted)]">
                            {f.file}:{f.line ?? 0}
                          </span>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </>
            )}
            {activeTab === "signature" && <SignatureTab preview={preview} />}
          </div>

          {/* Ack checkbox when required */}
          {needsPermAck && (
            <label className="flex items-start gap-2 text-xs cursor-pointer select-none">
              <input
                type="checkbox"
                checked={ackChecked}
                onChange={(e) => setAckChecked(e.target.checked)}
                className="mt-0.5 cursor-pointer"
                data-testid="ack-checkbox"
              />
              <span className="text-[var(--text)]">
                {t("admin.plugin.updateDialog.ackLabel")}
              </span>
            </label>
          )}

          {publisherBlock && (
            <label className="flex items-start gap-2 text-xs cursor-pointer select-none">
              <input
                type="checkbox"
                checked={publisherAck}
                onChange={(e) => setPublisherAck(e.target.checked)}
                className="mt-0.5 cursor-pointer"
                data-testid="publisher-ack-checkbox"
              />
              <span className="font-semibold" style={{ color: "var(--badge-error-text)" }}>
                {preview.signatureStatus === "signature-lost"
                  ? t("admin.plugin.signature.acceptSignatureLost")
                  : t("admin.plugin.signature.acceptNewPublisher")}
              </span>
            </label>
          )}

          {error && (
            <p
              className="text-sm rounded px-3 py-2"
              style={{
                color: "var(--badge-error-text)",
                background: "var(--badge-error-bg, rgba(127,29,29,0.15))",
              }}
              data-testid="error-message"
            >
              {error}
            </p>
          )}
        </div>
      )}

      {phase === "error" && preview === null && (
        <div className="flex flex-col gap-4 py-4">
          <p
            className="text-sm rounded px-3 py-2"
            style={{
              color: "var(--badge-error-text)",
              background: "var(--badge-error-bg, rgba(127,29,29,0.15))",
            }}
            data-testid="error-message"
          >
            {error ?? t("common.unknownError")}
          </p>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-4 border-t border-[var(--border)] mt-2">
        <Button variant="ghost" onClick={onClose} disabled={phase === "applying"}>
          {t("common.cancel")}
        </Button>
        <Button
          variant="primary"
          onClick={handleApply}
          disabled={applyDisabled}
          loading={phase === "applying"}
          data-testid="apply-button"
        >
          {t("admin.plugin.updateDialog.apply")}
        </Button>
      </div>
    </Modal>
  );
}
