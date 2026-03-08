import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
function stateBadgeVariant(state) {
    switch (state) {
        case "PLUGIN_STATE_RUNNING": return "success";
        case "PLUGIN_STATE_ERROR": return "error";
        case "PLUGIN_STATE_STOPPED": return "warning";
        case "PLUGIN_STATE_INSTALLED": return "info";
        default: return "default";
    }
}
function stateLabel(state) {
    switch (state) {
        case "PLUGIN_STATE_RUNNING": return "Running";
        case "PLUGIN_STATE_ERROR": return "Error";
        case "PLUGIN_STATE_STOPPED": return "Stopped";
        case "PLUGIN_STATE_INSTALLED": return "Installed";
        default: return "Unknown";
    }
}
export function PluginCard({ plugin, onEnable, onDisable, onUninstall }) {
    return (_jsxs(Card, { className: "flex flex-col gap-3", children: [_jsxs("div", { className: "flex items-start justify-between gap-2", children: [_jsxs("div", { className: "flex flex-col gap-0.5 min-w-0", children: [_jsx("span", { className: "font-semibold truncate", style: { color: "var(--text)" }, children: plugin.name }), _jsxs("span", { className: "text-xs", style: { color: "var(--text-muted)" }, children: ["v", plugin.version, " \u2014 ", plugin.language] })] }), _jsxs("div", { className: "flex items-center gap-2 shrink-0", children: [_jsx(Badge, { variant: stateBadgeVariant(plugin.state), children: stateLabel(plugin.state) }), plugin.enabled ? (_jsx(Badge, { variant: "success", children: "Enabled" })) : (_jsx(Badge, { variant: "default", children: "Disabled" }))] })] }), plugin.description && (_jsx("p", { className: "text-sm", style: { color: "var(--text-muted)" }, children: plugin.description })), _jsxs("div", { className: "flex gap-2 pt-1", children: [plugin.enabled ? (_jsx(Button, { variant: "secondary", onClick: onDisable, children: "Disable" })) : (_jsx(Button, { variant: "primary", onClick: onEnable, children: "Enable" })), _jsx(Button, { variant: "danger", onClick: onUninstall, children: "Uninstall" })] })] }));
}
