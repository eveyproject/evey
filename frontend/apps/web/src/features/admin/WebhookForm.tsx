import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useWebhookServiceCreateWebhook, useWebhookServiceUpdateWebhook } from "@evey/api";
import type { V1WebhookConfig } from "@evey/api";
import { WEBHOOK_EVENT_TYPES } from "@evey/client";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";


interface WebhookFormProps {
  onSuccess: () => void;
  existing?: V1WebhookConfig;
}

export function WebhookForm({ onSuccess, existing }: WebhookFormProps) {
  const { t } = useTranslation();
  const [name, setName] = useState(existing?.name ?? "");
  const [url, setUrl] = useState(existing?.url ?? "");
  const [secret, setSecret] = useState(existing?.secret ?? "");
  const [enabled, setEnabled] = useState(existing?.enabled ?? true);
  const [selectedEvents, setSelectedEvents] = useState<Set<string>>(
    new Set(existing?.events ?? ["*"]),
  );
  const [allEvents, setAllEvents] = useState(
    !existing || (existing.events?.length === 1 && existing.events[0] === "*"),
  );

  // onError surfaces a toast - without it a rejected save (e.g. the server's
  // SSRF URL validation refusing a private-IP webhook) failed silently.
  const createWebhook = useWebhookServiceCreateWebhook({
    mutation: { onSuccess, onError: (err: Error) => toast.error(err?.message || t("admin.webhooks.failedCreate")) },
  });
  const updateWebhook = useWebhookServiceUpdateWebhook({
    mutation: { onSuccess, onError: (err: Error) => toast.error(err?.message || t("admin.webhooks.failedUpdate")) },
  });

  const isPending = createWebhook.isPending || updateWebhook.isPending;

  function toggleEvent(evt: string) {
    setSelectedEvents((prev) => {
      const next = new Set(prev);
      if (next.has(evt)) next.delete(evt);
      else next.add(evt);
      return next;
    });
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const events = allEvents ? ["*"] : Array.from(selectedEvents);
    const payload = { name, url, events, secret: secret || undefined, enabled };

    if (existing?.id) {
      updateWebhook.mutate({ id: existing.id, data: payload });
    } else {
      createWebhook.mutate({ data: payload });
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        id="webhook-name"
        data-testid="webhook-name-input"
        label={t("admin.webhooks.form.name")}
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="My Webhook"
        required
      />
      <Input
        id="webhook-url"
        data-testid="webhook-url-input"
        label={t("admin.webhooks.form.url")}
        type="url"
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        placeholder="https://example.com/webhook"
        required
      />
      <Input
        label={t("admin.webhooks.form.secret")}
        value={secret}
        onChange={(e) => setSecret(e.target.value)}
        placeholder="mysecret"
      />

      <div>
        <p className="text-xs font-medium mb-2" style={{ color: "var(--text)" }}>{t("admin.webhooks.form.events")}</p>
        <label className="flex items-center gap-2 mb-2 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={allEvents}
            onChange={(e) => setAllEvents(e.target.checked)}
            className="accent-[var(--accent)]"
          />
          <span className="text-sm" style={{ color: "var(--text)" }}>{t("admin.webhooks.form.allEvents")}</span>
        </label>
        {!allEvents && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5 pl-2">
            {WEBHOOK_EVENT_TYPES.map((evt) => (
              <label key={evt} className="flex items-center gap-2 cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={selectedEvents.has(evt)}
                  onChange={() => toggleEvent(evt)}
                  className="accent-[var(--accent)]"
                />
                <span className="text-xs font-mono" style={{ color: "var(--text-muted)" }}>{evt}</span>
              </label>
            ))}
          </div>
        )}
      </div>

      <label className="flex items-center gap-2 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="accent-[var(--accent)]"
        />
        <span className="text-sm" style={{ color: "var(--text)" }}>{t("admin.webhooks.form.enabled")}</span>
      </label>

      <div className="flex justify-end pt-2">
        <Button type="submit" loading={isPending} disabled={!url.trim() || !name.trim()} data-testid="webhook-form-submit">
          {existing ? t("common.save") : t("admin.webhooks.form.createWebhook")}
        </Button>
      </div>
    </form>
  );
}
