import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { AdminNav } from "./index";
import { useQueryClient } from "@tanstack/react-query";
import {
  useWebhookServiceListWebhooks,
  useWebhookServiceDeleteWebhook,
} from "@evey/api";
import type { V1WebhookConfig } from "@evey/api/src/generated/webhook.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { WebhookForm } from "@/features/admin/WebhookForm";

export const Route = createFileRoute("/_auth/admin/webhooks")({
  component: WebhooksPage,
});

const WEBHOOKS_QUERY_KEY = ["/api/v1/admin/webhooks"];

function WebhookRow({ webhook }: { webhook: V1WebhookConfig }) {
  const queryClient = useQueryClient();

  const deleteWebhook = useWebhookServiceDeleteWebhook({
    mutation: {
      onSuccess: () => queryClient.invalidateQueries({ queryKey: WEBHOOKS_QUERY_KEY }),
    },
  });

  return (
    <tr style={{ borderBottom: "1px solid var(--border)" }}>
      <td className="px-4 py-3 max-w-xs truncate" style={{ color: "var(--text)" }}>
        {webhook.url}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {webhook.events?.join(", ") ?? "—"}
      </td>
      <td className="px-4 py-3">
        {webhook.enabled ? (
          <Badge variant="success">Enabled</Badge>
        ) : (
          <Badge variant="error">Disabled</Badge>
        )}
      </td>
      <td className="px-4 py-3 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
        {webhook.createdAt ? new Date(webhook.createdAt).toLocaleString() : "—"}
      </td>
      <td className="px-4 py-3">
        <Button
          variant="danger"
          loading={deleteWebhook.isPending}
          onClick={() => {
            if (confirm("Delete this webhook?")) {
              deleteWebhook.mutate({ id: webhook.id! });
            }
          }}
        >
          Delete
        </Button>
      </td>
    </tr>
  );
}

function WebhooksPage() {
  const [showModal, setShowModal] = useState(false);
  const queryClient = useQueryClient();
  const { data, isLoading } = useWebhookServiceListWebhooks();
  const webhooks = data?.webhooks ?? [];

  function handleCreateSuccess() {
    queryClient.invalidateQueries({ queryKey: WEBHOOKS_QUERY_KEY });
    setShowModal(false);
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold mb-2" style={{ color: "var(--text)" }}>
          Admin
        </h1>
        <AdminNav />
      </div>
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold" style={{ color: "var(--text)" }}>
          Webhooks
        </h2>
        <Button onClick={() => setShowModal(true)}>Create Webhook</Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)]"
          style={{ border: "1px solid var(--border)" }}
        >
          <table className="w-full text-sm">
            <thead>
              <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
                {["URL", "Events", "Status", "Created", "Actions"].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-medium"
                    style={{ color: "var(--text-muted)" }}
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
                    className="px-4 py-8 text-center"
                    style={{ color: "var(--text-muted)" }}
                  >
                    No webhooks configured.
                  </td>
                </tr>
              ) : (
                webhooks.map((wh) => <WebhookRow key={wh.id} webhook={wh} />)
              )}
            </tbody>
          </table>
        </div>
      )}

      <Modal open={showModal} onClose={() => setShowModal(false)} title="Create Webhook">
        <WebhookForm onSuccess={handleCreateSuccess} />
      </Modal>
    </div>
  );
}
