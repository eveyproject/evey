import { useState } from "react";
import { useWebhookServiceCreateWebhook } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

interface WebhookFormProps {
  onSuccess: () => void;
}

export function WebhookForm({ onSuccess }: WebhookFormProps) {
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState("*");

  const createWebhook = useWebhookServiceCreateWebhook({
    mutation: { onSuccess },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const eventList = events.trim() === "*"
      ? ["*"]
      : events.split(",").map((s) => s.trim()).filter(Boolean);
    createWebhook.mutate({
      data: { url, events: eventList, enabled: true },
    });
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      <Input
        label="URL"
        type="url"
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        placeholder="https://example.com/webhook"
        required
      />

      <Input
        label='Events ("*" for all, or comma-separated)'
        value={events}
        onChange={(e) => setEvents(e.target.value)}
        placeholder="* or media.created,library.scanned"
      />

      <div className="flex justify-end pt-2">
        <Button type="submit" loading={createWebhook.isPending} disabled={!url.trim()}>
          Create Webhook
        </Button>
      </div>
    </form>
  );
}
