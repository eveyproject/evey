import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useWebhookServiceListWebhooks,
  useWebhookServiceDeleteWebhook,
  useWebhookServiceUpdateWebhook,
  eveyFetch,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1WebhookConfig } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { WebhookForm } from "@/features/admin/WebhookForm";

export const Route = createLazyFileRoute("/_auth/admin/webhooks")({
  component: WebhooksPage,
});

const WEBHOOKS_QUERY_KEY = ["/api/v1/admin/webhooks"];

function WebhookRow({
  webhook,
  onEdit,
}: {
  webhook: V1WebhookConfig;
  onEdit: (wh: V1WebhookConfig) => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [testing, setTesting] = useState(false);

  const deleteWebhook = useWebhookServiceDeleteWebhook({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: WEBHOOKS_QUERY_KEY });
        toast.success(t("admin.webhooks.webhookDeleted"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.webhooks.failedDelete"))),
    },
  });

  const toggleEnabled = useWebhookServiceUpdateWebhook({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: WEBHOOKS_QUERY_KEY }),
      onError: () => toast.error(t("admin.webhooks.failedUpdate")),
    },
  });

  async function handleTest() {
    if (!webhook.id) return;
    setTesting(true);
    try {
      await eveyFetch<void>({
        url: `/api/v1/admin/webhooks/${webhook.id}/test`,
        method: "POST",
      });
      toast.success(t("admin.webhooks.testPingSent"));
    } catch {
      toast.error(t("admin.webhooks.testPingFailed"));
    } finally {
      setTesting(false);
    }
  }

  function handleToggleEnabled() {
    if (!webhook.id) return;
    toggleEnabled.mutate({
      id: webhook.id,
      data: {
        name: webhook.name,
        url: webhook.url,
        events: webhook.events,
        secret: webhook.secret,
        enabled: !webhook.enabled,
      },
    });
  }

  const eventTags = webhook.events ?? [];

  return (
    <tr className="border-b border-[var(--border)]">
      <td className="px-4 py-3 max-w-xs" data-label={t("admin.webhooks.tableHeaders.nameUrl")}>
        <div className="font-medium text-sm truncate text-[var(--text)]">
          {webhook.name || "\u2014"}
        </div>
        <div className="text-xs truncate mt-0.5 text-[var(--text-muted)]">
          {webhook.url}
        </div>
      </td>
      <td className="px-4 py-3" data-label={t("admin.webhooks.tableHeaders.events")}>
        <div className="flex flex-wrap gap-1">
          {eventTags.map((evt) => (
            <span
              key={evt}
              className="text-xs px-1.5 py-0.5 rounded font-mono bg-[var(--surface-raised)] text-[var(--text-muted)]"
            >
              {evt}
            </span>
          ))}
        </div>
      </td>
      <td className="px-4 py-3" data-label={t("admin.webhooks.tableHeaders.status")}>
        <button
          type="button"
          onClick={handleToggleEnabled}
          disabled={toggleEnabled.isPending}
          title={webhook.enabled ? t("common.disable") : t("common.enable")}
          aria-label={webhook.enabled ? t("common.disable") : t("common.enable")}
          className="cursor-pointer"
        >
          {webhook.enabled ? (
            <Badge variant="success">{t("common.enabled")}</Badge>
          ) : (
            <Badge variant="error">{t("common.disabled")}</Badge>
          )}
        </button>
      </td>
      <td className="px-4 py-3 whitespace-nowrap text-sm text-[var(--text-muted)]" data-label={t("admin.webhooks.tableHeaders.created")}>
        {webhook.createdAt ? new Date(webhook.createdAt).toLocaleString() : "\u2014"}
      </td>
      <td className="px-4 py-3" data-label={t("admin.webhooks.tableHeaders.actions")}>
        <div className="flex gap-2 flex-wrap">
          <Button variant="secondary" loading={testing} onClick={handleTest}>
            {t("common.test")}
          </Button>
          <Button variant="secondary" onClick={() => onEdit(webhook)}>
            {t("common.edit")}
          </Button>
          <Button
            variant="danger"
            loading={deleteWebhook.isPending}
            onClick={() => setShowDeleteConfirm(true)}
          >
            {t("common.delete")}
          </Button>
        </div>
        <Modal
          open={showDeleteConfirm}
          onClose={() => setShowDeleteConfirm(false)}
          title={t("admin.webhooks.deleteTitle")}
        >
          <p className="text-sm mb-4 text-[var(--text-muted)]">
            {t("admin.webhooks.deleteConfirm", { name: webhook.name || webhook.url })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowDeleteConfirm(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              loading={deleteWebhook.isPending}
              onClick={() => {
                deleteWebhook.mutate({ id: webhook.id! });
                setShowDeleteConfirm(false);
              }}
            >
              {t("common.delete")}
            </Button>
          </div>
        </Modal>
      </td>
    </tr>
  );
}

function WebhooksPage() {
  const { t } = useTranslation();
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editTarget, setEditTarget] = useState<V1WebhookConfig | null>(null);
  const queryClient = useQueryClient();
  const { data, isLoading } = useWebhookServiceListWebhooks();
  const webhooks = data?.webhooks ?? [];

  function handleMutationSuccess() {
    queryClient.invalidateQueries({ queryKey: WEBHOOKS_QUERY_KEY });
    setShowCreateModal(false);
    setEditTarget(null);
    toast.success(editTarget ? t("admin.webhooks.webhookUpdated") : t("admin.webhooks.webhookCreated"));
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-[var(--text)]">
          {t("admin.webhooks.title")}
        </h2>
        <Button onClick={() => setShowCreateModal(true)} data-testid="add-webhook-btn">{t("admin.webhooks.createWebhook")}</Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
        >
          <table className="w-full text-sm table-cards">
            <thead>
              <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                {[t("admin.webhooks.tableHeaders.nameUrl"), t("admin.webhooks.tableHeaders.events"), t("admin.webhooks.tableHeaders.status"), t("admin.webhooks.tableHeaders.created"), t("admin.webhooks.tableHeaders.actions")].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {webhooks.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="px-4 py-8 text-center text-[var(--text-muted)]"
                  >
                    {t("admin.webhooks.noWebhooks")}
                  </td>
                </tr>
              ) : (
                webhooks.map((wh) => (
                  <WebhookRow key={wh.id} webhook={wh} onEdit={setEditTarget} />
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      <Modal
        open={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        title={t("admin.webhooks.createWebhook")}
      >
        <WebhookForm onSuccess={handleMutationSuccess} />
      </Modal>

      <Modal
        open={!!editTarget}
        onClose={() => setEditTarget(null)}
        title={t("admin.webhooks.editWebhook")}
      >
        {editTarget && (
          <WebhookForm existing={editTarget} onSuccess={handleMutationSuccess} />
        )}
      </Modal>
    </div>
  );
}
