import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useUserServiceListAPIKeys,
  useUserServiceCreateAPIKey,
  useUserServiceRevokeAPIKey,
  getUserServiceListAPIKeysQueryKey,
} from "@evey/api";
import type { V1APIKey } from "@evey/api/src/generated/user.schemas";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";

function maskKey(key: string): string {
  if (key.length <= 4) return "\u2022\u2022\u2022\u2022";
  return `evey_${"\u2022".repeat(8)}${key.slice(-4)}`;
}

function formatDate(iso?: string): string {
  if (!iso) return "\u2014";
  return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

interface CreatedKey {
  id: string;
  name: string;
  key: string;
}

export function APIKeySettings() {
  const { t } = useTranslation("auth");
  const { t: tc } = useTranslation("common");
  const qc = useQueryClient();
  const { data, isLoading } = useUserServiceListAPIKeys();
  const keys = data?.apiKeys ?? [];

  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [nameError, setNameError] = useState("");
  const [createdKey, setCreatedKey] = useState<CreatedKey | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<V1APIKey | null>(null);
  const [copied, setCopied] = useState(false);

  const createMutation = useUserServiceCreateAPIKey({
    mutation: {
      onSuccess: (key) => {
        void qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
        setCreateOpen(false);
        setNewName("");
        setNameError("");
        if (key.key && key.id && key.name) {
          setCreatedKey({ id: key.id, name: key.name, key: key.key });
        }
      },
      onError: (err: Error) => toast.error(err.message || t("apiKeys.createError")),
    },
  });

  const revokeMutation = useUserServiceRevokeAPIKey({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
        setRevokeTarget(null);
        toast.success(t("apiKeys.keyRevoked"));
      },
      onError: (err: Error) => toast.error(err.message || t("apiKeys.revokeError")),
    },
  });

  function handleCreate() {
    if (!newName.trim()) {
      setNameError(t("apiKeys.nameRequired"));
      return;
    }
    setNameError("");
    createMutation.mutate({ data: { name: newName.trim() } });
  }

  function handleCopy() {
    if (!createdKey) return;
    void navigator.clipboard.writeText(createdKey.key).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("apiKeys.title")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t("apiKeys.description")}
          </p>
        </div>
        <Button variant="primary" onClick={() => setCreateOpen(true)}>
          {tc("newKey")}
        </Button>
      </div>

      {isLoading ? (
        <div className="text-sm text-[var(--text-muted)]">{tc("loading")}</div>
      ) : keys.length === 0 ? (
        <div className="text-sm text-[var(--text-muted)]">{t("apiKeys.noKeys")}</div>
      ) : (
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="text-left text-xs text-[var(--text-muted)] border-b border-[var(--border)]">
              <th className="pb-2 font-semibold">{t("apiKeys.name")}</th>
              <th className="pb-2 font-semibold">{t("apiKeys.key")}</th>
              <th className="pb-2 font-semibold">{t("apiKeys.created")}</th>
              <th className="pb-2 font-semibold">{t("apiKeys.lastUsed")}</th>
              <th className="pb-2" />
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.id} className="border-b border-[var(--border)] last:border-0">
                <td className="py-3 pr-4 font-medium text-[var(--text)]">{k.name}</td>
                <td className="py-3 pr-4 font-mono text-[var(--text-muted)]">
                  {k.key ? maskKey(k.key) : "evey_\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022"}
                </td>
                <td className="py-3 pr-4 text-[var(--text-muted)]">{formatDate(k.createdAt)}</td>
                <td className="py-3 pr-4 text-[var(--text-muted)]">{formatDate(k.lastUsedAt)}</td>
                <td className="py-3">
                  <Button
                    variant="danger"
                    className="py-1 px-2 text-xs"
                    onClick={() => setRevokeTarget(k)}
                  >
                    {tc("revoke")}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Create dialog */}
      <Modal open={createOpen} onClose={() => { setCreateOpen(false); setNewName(""); setNameError(""); }} title={t("apiKeys.newKeyTitle")}>
        <div className="flex flex-col gap-4">
          <Input
            label={t("apiKeys.name")}
            placeholder={t("apiKeys.namePlaceholder")}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            error={nameError}
            onKeyDown={(e) => { if (e.key === "Enter") handleCreate(); }}
            autoFocus
          />
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="secondary" onClick={() => { setCreateOpen(false); setNewName(""); setNameError(""); }}>
              {tc("cancel")}
            </Button>
            <Button variant="primary" loading={createMutation.isPending} onClick={handleCreate}>
              {tc("create")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Show new key once */}
      <Modal open={!!createdKey} onClose={() => { setCreatedKey(null); setCopied(false); }} title={t("apiKeys.keyCreatedTitle")}>
        <div className="flex flex-col gap-4">
          <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] px-3 py-2">
            <p className="text-xs text-[var(--text-muted)] mb-1 font-medium">{createdKey?.name}</p>
            <p className="font-mono text-sm break-all text-[var(--text)]">{createdKey?.key}</p>
          </div>
          <div className="rounded-[var(--radius-md)] px-3 py-2 text-xs" style={{ backgroundColor: "var(--badge-warning-bg)", borderColor: "var(--toast-warning-border)", borderWidth: "1px", borderStyle: "solid", color: "var(--badge-warning-text)" }}>
            {t("apiKeys.saveWarning")}
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => { setCreatedKey(null); setCopied(false); }}>
              {tc("done")}
            </Button>
            <Button variant="primary" onClick={handleCopy}>
              {copied ? tc("copied") : tc("copyKey")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Revoke confirmation */}
      <Modal open={!!revokeTarget} onClose={() => setRevokeTarget(null)} title={t("apiKeys.revokeTitle")}>
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text)]"
            dangerouslySetInnerHTML={{
              __html: t("apiKeys.revokeConfirm", { name: revokeTarget?.name ?? "" }),
            }}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setRevokeTarget(null)}>
              {tc("cancel")}
            </Button>
            <Button
              variant="danger"
              loading={revokeMutation.isPending}
              onClick={() => { if (revokeTarget?.id) revokeMutation.mutate({ id: revokeTarget.id }); }}
            >
              {tc("revoke")}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
