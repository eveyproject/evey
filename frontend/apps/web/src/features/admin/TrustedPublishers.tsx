import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceListTrustedPublishers,
  useAdminPluginServiceAddTrustedPublisher,
  useAdminPluginServiceRemoveTrustedPublisher,
  getAdminPluginServiceListTrustedPublishersQueryKey,
  getAdminPluginServiceListPluginsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import { Modal } from "@/components/ui/Modal";
import { Spinner } from "@/components/ui/Spinner";
import { useDemoLockFor } from "@/hooks/useDemoMode";
import { shortFingerprint } from "./pluginSignature";

interface TrustedPublisher {
  fingerprint?: string;
  name?: string;
  addedAt?: string;
  seeded?: boolean;
}

// The instance's keyring: plugins signed by one of these keys show as trusted.
export function TrustedPublishers() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const addLock = useDemoLockFor("/evey.v1.AdminPluginService/AddTrustedPublisher").lock;
  const removeLock = useDemoLockFor("/evey.v1.AdminPluginService/RemoveTrustedPublisher").lock;
  const { data, isLoading } = useAdminPluginServiceListTrustedPublishers();
  const publishers = data?.publishers ?? [];
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [pem, setPem] = useState("");
  const [removeTarget, setRemoveTarget] = useState<TrustedPublisher | null>(null);

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListTrustedPublishersQueryKey() });
    // Cards re-derive their trusted badge from the list.
    void queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
  };

  const addPublisher = useAdminPluginServiceAddTrustedPublisher({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.publishers.added"));
        setAdding(false);
        setName("");
        setPem("");
        refresh();
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.publishers.addFailed"))),
    },
  });
  const removePublisher = useAdminPluginServiceRemoveTrustedPublisher({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.publishers.removed"));
        setRemoveTarget(null);
        refresh();
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.publishers.removeFailed"))),
    },
  });

  return (
    <div className="flex flex-col gap-3" data-testid="trusted-publishers">
      <div className="flex items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("admin.plugin.publishers.title")}</h3>
          <p className="text-xs text-[var(--text-muted)]">{t("admin.plugin.publishers.description")}</p>
        </div>
        <Button variant="secondary" onClick={() => setAdding(true)} data-testid="publisher-add-open" {...addLock}>
          {t("admin.plugin.publishers.add")}
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-4"><Spinner /></div>
      ) : publishers.length === 0 ? (
        <p className="text-xs text-[var(--text-muted)]">{t("admin.plugin.publishers.empty")}</p>
      ) : (
        <ul className="flex flex-col divide-y divide-[var(--border)] rounded-[var(--radius-md)] border border-[var(--border)]">
          {publishers.map((p) => (
            <li key={p.fingerprint} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2" data-testid="publisher-row">
              <span className="text-sm font-medium text-[var(--text)]">{p.name}</span>
              {p.seeded && <Badge variant="info" data-testid="publisher-seeded">{t("admin.plugin.publishers.seeded")}</Badge>}
              <span className="font-mono text-xs text-[var(--text-muted)]" title={p.fingerprint}>
                {shortFingerprint(p.fingerprint)}
              </span>
              {p.addedAt && (
                <span className="text-xs text-[var(--text-muted)]">
                  {new Date(p.addedAt).toLocaleDateString()}
                </span>
              )}
              {!p.seeded && (
                <Button
                  variant="ghost"
                 
                  className="ml-auto"
                  onClick={() => setRemoveTarget(p)}
                  data-testid="publisher-remove"
                  {...removeLock}
                >
                  {t("admin.plugin.publishers.remove")}
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}

      <Modal open={adding} onClose={() => setAdding(false)} title={t("admin.plugin.publishers.addTitle")}>
        <div className="flex flex-col gap-3">
          <Input label={t("admin.plugin.publishers.nameLabel")} value={name} onChange={(e) => setName(e.target.value)} />
          <Textarea
            label={t("admin.plugin.publishers.pemLabel")}
            value={pem}
            onChange={(e) => setPem(e.target.value)}
            placeholder={t("admin.plugin.publishers.pemPlaceholder")}
            className="font-mono text-xs"
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setAdding(false)}>{t("common.cancel")}</Button>
            <Button
              variant="primary"
              disabled={!name.trim() || !pem.trim()}
              loading={addPublisher.isPending}
              onClick={() => addPublisher.mutate({ data: { name: name.trim(), publicKeyPem: pem.trim() } })}
              data-testid="publisher-add-confirm"
            >
              {t("admin.plugin.publishers.add")}
            </Button>
          </div>
        </div>
      </Modal>

      <Modal open={!!removeTarget} onClose={() => setRemoveTarget(null)} title={t("admin.plugin.publishers.remove")}>
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {t("admin.plugin.publishers.removeConfirm", { name: removeTarget?.name })}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setRemoveTarget(null)}>{t("common.cancel")}</Button>
          <Button
            variant="danger"
            loading={removePublisher.isPending}
            onClick={() => removeTarget?.fingerprint && removePublisher.mutate({ fingerprint: removeTarget.fingerprint })}
            data-testid="publisher-remove-confirm"
          >
            {t("admin.plugin.publishers.remove")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
