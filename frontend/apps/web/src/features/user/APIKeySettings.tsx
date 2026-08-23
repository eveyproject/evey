import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { BookOpen } from "lucide-react";
import {
  useUserServiceListAPIKeys,
  useUserServiceCreateAPIKey,
  useUserServiceRevokeAPIKey,
  getUserServiceListAPIKeysQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1APIKey } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";

const SCOPE_OPTIONS = [
  "media:read",
  "media:write",
  "admin:read",
  "admin:write",
  "users:read",
  "users:write",
  "playback:read",
  "playback:write",
] as const;

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
  const { t } = useTranslation();
    const qc = useQueryClient();
  const { data, isLoading } = useUserServiceListAPIKeys();
  const keys = data?.apiKeys ?? [];

  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [nameError, setNameError] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<Set<string>>(new Set());
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
        setSelectedScopes(new Set());
        if (key.key && key.id && key.name) {
          setCreatedKey({ id: key.id, name: key.name, key: key.key });
        }
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("auth.apiKeys.createError"))),
    },
  });

  const revokeMutation = useUserServiceRevokeAPIKey({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListAPIKeysQueryKey() });
        setRevokeTarget(null);
        toast.success(t("auth.apiKeys.keyRevoked"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("auth.apiKeys.revokeError"))),
    },
  });

  function handleCreate() {
    if (!newName.trim()) {
      setNameError(t("auth.apiKeys.nameRequired"));
      return;
    }
    setNameError("");
    createMutation.mutate({
      data: { name: newName.trim(), scopes: Array.from(selectedScopes) },
    });
  }

  function toggleScope(scope: string) {
    setSelectedScopes((prev) => {
      const next = new Set(prev);
      if (next.has(scope)) next.delete(scope);
      else next.add(scope);
      return next;
    });
  }

  function handleCopy() {
    if (!createdKey) return;
    navigator.clipboard.writeText(createdKey.key).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      },
      () => toast.error(t("common.clipboardFailed")),
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.apiKeys.title")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t("auth.apiKeys.description")}
          </p>
          <Link
            to="/admin/api-docs"
            className="inline-flex items-center gap-1 text-xs text-[var(--accent)] hover:underline mt-2"
          >
            <BookOpen size={13} aria-hidden="true" />
            {t("auth.apiDocs.link")}
          </Link>
        </div>
        <Button variant="primary" onClick={() => setCreateOpen(true)}>
          {t("common.newKey")}
        </Button>
      </div>

      {isLoading ? (
        <div className="text-sm text-[var(--text-muted)]">{t("common.loading")}</div>
      ) : keys.length === 0 ? (
        <div className="text-sm text-[var(--text-muted)]">{t("auth.apiKeys.noKeys")}</div>
      ) : (
        <table className="w-full text-sm border-collapse">
          <thead>
            <tr className="text-left text-xs text-[var(--text-muted)] border-b border-[var(--border)]">
              <th className="pb-2 font-semibold">{t("auth.apiKeys.name")}</th>
              <th className="pb-2 font-semibold">{t("auth.apiKeys.key")}</th>
              <th className="pb-2 font-semibold">{t("auth.apiKeys.scopes")}</th>
              <th className="pb-2 font-semibold">{t("auth.apiKeys.created")}</th>
              <th className="pb-2 font-semibold">{t("auth.apiKeys.lastUsed")}</th>
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
                <td className="py-3 pr-4">
                  <div className="flex flex-wrap gap-1">
                    {(k.scopes ?? []).length === 0 ? (
                      <span className="text-xs text-[var(--text-muted)]">{t("auth.apiKeys.scopesAll")}</span>
                    ) : (
                      (k.scopes ?? []).map((sc) => (
                        <span key={sc} className="text-xs px-1.5 py-0.5 rounded font-mono bg-[var(--surface-raised)] text-[var(--text-muted)]">
                          {sc}
                        </span>
                      ))
                    )}
                  </div>
                </td>
                <td className="py-3 pr-4 text-[var(--text-muted)]">{formatDate(k.createdAt)}</td>
                <td className="py-3 pr-4 text-[var(--text-muted)]">{formatDate(k.lastUsedAt)}</td>
                <td className="py-3">
                  <Button
                    variant="danger"
                    className="py-1 px-2 text-xs"
                    onClick={() => setRevokeTarget(k)}
                  >
                    {t("common.revoke")}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Create dialog */}
      <Modal open={createOpen} onClose={() => { setCreateOpen(false); setNewName(""); setNameError(""); setSelectedScopes(new Set()); }} title={t("auth.apiKeys.newKeyTitle")}>
        <div className="flex flex-col gap-4">
          <Input
            label={t("auth.apiKeys.name")}
            placeholder={t("auth.apiKeys.namePlaceholder")}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            error={nameError}
            onKeyDown={(e) => { if (e.key === "Enter") handleCreate(); }}
            autoFocus
          />
          <div>
            <p className="text-xs font-medium mb-2 text-[var(--text)]">{t("auth.apiKeys.scopes")}</p>
            <p className="text-xs text-[var(--text-muted)] mb-2">{t("auth.apiKeys.scopesHint")}</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5 pl-2">
              {SCOPE_OPTIONS.map((sc) => (
                <label key={sc} className="flex items-center gap-2 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={selectedScopes.has(sc)}
                    onChange={() => toggleScope(sc)}
                    className="accent-[var(--accent)]"
                  />
                  <span className="text-xs font-mono text-[var(--text-muted)]">{sc}</span>
                  <span className="text-xs text-[var(--text-muted)]">- {t(`auth.apiKeys.scopeLabels.${sc}`)}</span>
                </label>
              ))}
            </div>
          </div>
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="secondary" onClick={() => { setCreateOpen(false); setNewName(""); setNameError(""); setSelectedScopes(new Set()); }}>
              {t("common.cancel")}
            </Button>
            <Button variant="primary" loading={createMutation.isPending} onClick={handleCreate}>
              {t("common.create")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Show new key once */}
      <Modal open={!!createdKey} onClose={() => { setCreatedKey(null); setCopied(false); }} title={t("auth.apiKeys.keyCreatedTitle")}>
        <div className="flex flex-col gap-4">
          <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] px-3 py-2">
            <p className="text-xs text-[var(--text-muted)] mb-1 font-medium">{createdKey?.name}</p>
            <p className="font-mono text-sm break-all text-[var(--text)]">{createdKey?.key}</p>
          </div>
          <div className="rounded-[var(--radius-md)] px-3 py-2 text-xs bg-[var(--badge-warning-bg)] border border-[var(--toast-warning-border)] text-[var(--badge-warning-text)]">
            {t("auth.apiKeys.saveWarning")}
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => { setCreatedKey(null); setCopied(false); }}>
              {t("common.done")}
            </Button>
            <Button variant="primary" onClick={handleCopy}>
              {copied ? t("common.copied") : t("common.copyKey")}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Revoke confirmation */}
      <Modal open={!!revokeTarget} onClose={() => setRevokeTarget(null)} title={t("auth.apiKeys.revokeTitle")}>
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text)]">
            {t("auth.apiKeys.revokeConfirm", { name: revokeTarget?.name ?? "" })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setRevokeTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              loading={revokeMutation.isPending}
              onClick={() => { if (revokeTarget?.id) revokeMutation.mutate({ id: revokeTarget.id }); }}
            >
              {t("common.revoke")}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
