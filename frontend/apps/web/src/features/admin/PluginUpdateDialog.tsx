import { useState, useEffect } from "react";
import { eveyFetch } from "@evey/api";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";

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
  requiresAck?: boolean;
  sourceHash?: string;
}

type Phase = "previewing" | "reviewing" | "applying" | "error";

type TabKey = "permissions" | "slots" | "externalFetch" | "findings" | "signature";

const TAB_LABELS: Record<TabKey, string> = {
  permissions: "Permissions",
  slots: "Slots",
  externalFetch: "Appels externes",
  findings: "Alertes",
  signature: "Signature",
};

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

export type Props = {
  pluginId: string;
  pluginName?: string;
  source: string;
  onClose: () => void;
  onApplied: () => void;
};

export function PluginUpdateDialog({ pluginId, pluginName, source, onClose, onApplied }: Props) {
  const [phase, setPhase] = useState<Phase>("previewing");
  const [preview, setPreview] = useState<PreviewResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<TabKey>("permissions");
  const [ackChecked, setAckChecked] = useState(false);

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
          const msg = err instanceof Error ? err.message : "Erreur lors du chargement de la prévisualisation";
          setError(msg);
          setPhase("error");
        }
      }
    }
    void doPreview();
    return () => { cancelled = true; };
  }, [pluginId, source]);

  async function handleApply() {
    if (!preview?.sourceHash) return;
    setPhase("applying");
    setError(null);
    try {
      await eveyFetch<unknown>({
        url: `/api/v1/admin/plugins/${pluginId}/apply-update`,
        method: "POST",
        data: { source, acknowledged_hash: preview.sourceHash },
      });
      onApplied();
      onClose();
    } catch (err: unknown) {
      // HTTP 409 or gRPC Aborted → candidate changed between preview and apply
      const isAborted =
        (err instanceof Error && (err.message.includes("409") || err.message.toLowerCase().includes("aborted"))) ||
        (typeof err === "object" && err !== null && "status" in err && (err as { status: number }).status === 409);
      const msg = isAborted
        ? "Le hash du candidat ne correspond plus — actualisez la prévisualisation"
        : err instanceof Error
          ? err.message
          : "Erreur lors de l'application de la mise à jour";
      setError(msg);
      setPhase("reviewing");
    }
  }

  const title = preview
    ? `Mise à jour ${pluginName ?? pluginId} : ${preview.currentVersion ?? "?"} → ${preview.candidateVersion ?? "?"}`
    : `Mise à jour ${pluginName ?? pluginId}`;

  const applyDisabled =
    phase === "previewing" ||
    phase === "applying" ||
    phase === "error" && preview === null ||
    (preview?.requiresAck && !ackChecked);

  return (
    <Modal open onClose={onClose} title={title} size="xl">
      {phase === "previewing" && (
        <div className="flex flex-col items-center gap-3 py-8">
          <Spinner size="md" />
          <p className="text-sm text-[var(--text-muted)]">Analyse du candidat en cours…</p>
        </div>
      )}

      {(phase === "reviewing" || phase === "applying" || (phase === "error" && preview !== null)) && preview && (
        <div className="flex flex-col gap-4">
          {/* Security banner */}
          {preview.requiresAck ? (
            <div
              className="rounded border px-3 py-2 text-sm font-semibold"
              style={{
                borderColor: "var(--color-danger)",
                background: "var(--badge-error-bg, rgba(127,29,29,0.15))",
                color: "var(--badge-error-text)",
              }}
              data-testid="banner-requires-ack"
            >
              Cette mise à jour demande de nouvelles permissions — vérifiez avant d'appliquer
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
              Aucun changement de permissions — mise à jour sûre
            </div>
          )}

          {/* Tabs */}
          <div className="flex gap-1 border-b border-[var(--border)]">
            {(Object.keys(TAB_LABELS) as TabKey[]).map((tab) => (
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
                {TAB_LABELS[tab]}
              </button>
            ))}
          </div>

          {/* Tab content */}
          <div className="min-h-[100px]">
            {activeTab === "permissions" && (
              <DiffList
                added={preview.permissionsAdded}
                removed={preview.permissionsRemoved}
                emptyLabel="Aucun changement de permissions"
              />
            )}
            {activeTab === "slots" && (
              <DiffList
                added={preview.slotsAdded}
                removed={preview.slotsRemoved}
                emptyLabel="Aucun changement de slots"
              />
            )}
            {activeTab === "externalFetch" && (
              <DiffList
                added={preview.externalFetchAdded}
                removed={preview.externalFetchRemoved}
                emptyLabel="Aucun changement d'appels externes"
              />
            )}
            {activeTab === "findings" && (
              <>
                {(preview.newFindings?.length ?? 0) === 0 ? (
                  <p className="text-xs text-[var(--text-muted)] py-2">Aucune alerte</p>
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
            {activeTab === "signature" && (
              <div className="py-2">
                {preview.signatureStatus === "signed" || preview.signatureStatus === "SIGNED" ? (
                  <Badge variant="success">Signé</Badge>
                ) : preview.signatureStatus === "invalid" || preview.signatureStatus === "INVALID" ? (
                  <Badge variant="error">Invalide</Badge>
                ) : (
                  <Badge variant="warning">Non signé</Badge>
                )}
              </div>
            )}
          </div>

          {/* Ack checkbox when required */}
          {preview.requiresAck && (
            <label className="flex items-start gap-2 text-xs cursor-pointer select-none">
              <input
                type="checkbox"
                checked={ackChecked}
                onChange={(e) => setAckChecked(e.target.checked)}
                className="mt-0.5 cursor-pointer"
                data-testid="ack-checkbox"
              />
              <span className="text-[var(--text)]">
                J'ai examiné les nouvelles permissions et j'accepte de les accorder à ce plugin
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
            {error ?? "Erreur inconnue"}
          </p>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-4 border-t border-[var(--border)] mt-2">
        <Button variant="ghost" onClick={onClose} disabled={phase === "applying"}>
          Annuler
        </Button>
        <Button
          variant="primary"
          onClick={handleApply}
          disabled={applyDisabled}
          loading={phase === "applying"}
          data-testid="apply-button"
        >
          Appliquer la mise à jour
        </Button>
      </div>
    </Modal>
  );
}
