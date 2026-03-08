import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { useWebhookServiceCreateWebhook } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
export function WebhookForm({ onSuccess }) {
    const [url, setUrl] = useState("");
    const [events, setEvents] = useState("*");
    const createWebhook = useWebhookServiceCreateWebhook({
        mutation: { onSuccess },
    });
    function handleSubmit(e) {
        e.preventDefault();
        const eventList = events.trim() === "*"
            ? ["*"]
            : events.split(",").map((s) => s.trim()).filter(Boolean);
        createWebhook.mutate({
            data: { url, events: eventList, enabled: true },
        });
    }
    return (_jsxs("form", { onSubmit: handleSubmit, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "URL", type: "url", value: url, onChange: (e) => setUrl(e.target.value), placeholder: "https://example.com/webhook", required: true }), _jsx(Input, { label: 'Events ("*" for all, or comma-separated)', value: events, onChange: (e) => setEvents(e.target.value), placeholder: "* or media.created,library.scanned" }), _jsx("div", { className: "flex justify-end pt-2", children: _jsx(Button, { type: "submit", loading: createWebhook.isPending, disabled: !url.trim(), children: "Create Webhook" }) })] }));
}
