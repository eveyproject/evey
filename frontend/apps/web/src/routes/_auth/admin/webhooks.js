import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useWebhookServiceListWebhooks, useWebhookServiceDeleteWebhook, } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { WebhookForm } from "@/features/admin/WebhookForm";
export const Route = createFileRoute("/_auth/admin/webhooks")({
    component: WebhooksPage,
});
const WEBHOOKS_QUERY_KEY = ["/api/v1/admin/webhooks"];
function WebhookRow({ webhook }) {
    const queryClient = useQueryClient();
    const deleteWebhook = useWebhookServiceDeleteWebhook({
        mutation: {
            onSuccess: () => queryClient.invalidateQueries({ queryKey: WEBHOOKS_QUERY_KEY }),
        },
    });
    return (_jsxs("tr", { style: { borderBottom: "1px solid var(--border)" }, children: [_jsx("td", { className: "px-4 py-3 max-w-xs truncate", style: { color: "var(--text)" }, children: webhook.url }), _jsx("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: webhook.events?.join(", ") ?? "—" }), _jsx("td", { className: "px-4 py-3", children: webhook.enabled ? (_jsx(Badge, { variant: "success", children: "Enabled" })) : (_jsx(Badge, { variant: "error", children: "Disabled" })) }), _jsx("td", { className: "px-4 py-3 whitespace-nowrap", style: { color: "var(--text-muted)" }, children: webhook.createdAt ? new Date(webhook.createdAt).toLocaleString() : "—" }), _jsx("td", { className: "px-4 py-3", children: _jsx(Button, { variant: "danger", loading: deleteWebhook.isPending, onClick: () => {
                        if (confirm("Delete this webhook?")) {
                            deleteWebhook.mutate({ id: webhook.id });
                        }
                    }, children: "Delete" }) })] }));
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
    return (_jsxs("div", { className: "flex flex-col gap-6", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: "Webhooks" }), _jsx(Button, { onClick: () => setShowModal(true), children: "Create Webhook" })] }), isLoading ? (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })) : (_jsx("div", { className: "overflow-x-auto rounded-[var(--radius-lg)]", style: { border: "1px solid var(--border)" }, children: _jsxs("table", { className: "w-full text-sm", children: [_jsx("thead", { children: _jsx("tr", { style: { backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }, children: ["URL", "Events", "Status", "Created", "Actions"].map((h) => (_jsx("th", { className: "px-4 py-2 text-left font-medium", style: { color: "var(--text-muted)" }, children: h }, h))) }) }), _jsx("tbody", { children: webhooks.length === 0 ? (_jsx("tr", { children: _jsx("td", { colSpan: 5, className: "px-4 py-8 text-center", style: { color: "var(--text-muted)" }, children: "No webhooks configured." }) })) : (webhooks.map((wh) => _jsx(WebhookRow, { webhook: wh }, wh.id))) })] }) })), _jsx(Modal, { open: showModal, onClose: () => setShowModal(false), title: "Create Webhook", children: _jsx(WebhookForm, { onSuccess: handleCreateSuccess }) })] }));
}
