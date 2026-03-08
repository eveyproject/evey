import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useAdminServiceListPlugins, useAdminServiceEnablePlugin, useAdminServiceDisablePlugin, useAdminServiceUninstallPlugin, useAdminServiceInstallPlugin, } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { PluginCard } from "@/features/admin/PluginCard";
export const Route = createFileRoute("/_auth/admin/plugins")({
    component: PluginsPage,
});
const PLUGINS_QUERY_KEY = ["/api/v1/admin/plugins"];
function InstallPluginForm({ onSuccess }) {
    const [source, setSource] = useState("");
    const [name, setName] = useState("");
    const installPlugin = useAdminServiceInstallPlugin({ mutation: { onSuccess } });
    function handleSubmit(e) {
        e.preventDefault();
        installPlugin.mutate({ data: { source, name } });
    }
    return (_jsxs("form", { onSubmit: handleSubmit, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "Source URL", value: source, onChange: (e) => setSource(e.target.value), placeholder: "https://example.com/plugin.zip", required: true }), _jsx(Input, { label: "Name (optional)", value: name, onChange: (e) => setName(e.target.value), placeholder: "my-plugin" }), _jsx("div", { className: "flex justify-end pt-2", children: _jsx(Button, { type: "submit", loading: installPlugin.isPending, disabled: !source.trim(), children: "Install" }) })] }));
}
function PluginsPage() {
    const [showModal, setShowModal] = useState(false);
    const queryClient = useQueryClient();
    const { data, isLoading } = useAdminServiceListPlugins();
    const plugins = data?.plugins ?? [];
    const invalidate = () => queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY });
    const enablePlugin = useAdminServiceEnablePlugin({ mutation: { onSuccess: invalidate } });
    const disablePlugin = useAdminServiceDisablePlugin({ mutation: { onSuccess: invalidate } });
    const uninstallPlugin = useAdminServiceUninstallPlugin({ mutation: { onSuccess: invalidate } });
    function handleInstallSuccess() {
        invalidate();
        setShowModal(false);
    }
    return (_jsxs("div", { className: "flex flex-col gap-6", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: "Plugins" }), _jsx(Button, { onClick: () => setShowModal(true), children: "Install Plugin" })] }), isLoading ? (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })) : plugins.length === 0 ? (_jsx("div", { className: "text-center py-16 rounded-[var(--radius-lg)]", style: { backgroundColor: "var(--surface)", border: "1px solid var(--border)" }, children: _jsx("p", { style: { color: "var(--text-muted)" }, children: "No plugins installed." }) })) : (_jsx("div", { className: "grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4", children: plugins.map((plugin) => (_jsx(PluginCard, { plugin: plugin, onEnable: () => enablePlugin.mutate({ id: plugin.id }), onDisable: () => disablePlugin.mutate({ id: plugin.id }), onUninstall: () => {
                        if (confirm(`Uninstall plugin "${plugin.name}"?`)) {
                            uninstallPlugin.mutate({ id: plugin.id });
                        }
                    } }, plugin.id))) })), _jsx(Modal, { open: showModal, onClose: () => setShowModal(false), title: "Install Plugin", children: _jsx(InstallPluginForm, { onSuccess: handleInstallSuccess }) })] }));
}
