import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser } from "@evey/api";
import { getUserServiceGetCurrentUserQueryKey } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { ThemeSettings } from "@/features/user/ThemeSettings";
import { NotificationSettings } from "@/features/user/NotificationSettings";
export const Route = createFileRoute("/_auth/profile/settings")({
    component: SettingsPage,
});
function SettingsPage() {
    const [tab, setTab] = useState("appearance");
    return (_jsxs("div", { className: "px-6 py-8 max-w-2xl flex flex-col gap-6", children: [_jsx("h1", { className: "text-2xl font-bold text-[var(--text)]", children: "Settings" }), _jsx("div", { className: "flex gap-1 border-b border-[var(--border)]", children: ["appearance", "notifications", "account"].map((t) => (_jsx("button", { onClick: () => setTab(t), className: `px-4 py-2 text-sm capitalize transition-colors border-b-2 -mb-px ${tab === t
                        ? "border-[var(--accent)] text-[var(--accent)]"
                        : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"}`, children: t }, t))) }), _jsx(Card, { children: _jsxs("div", { className: "p-6", children: [tab === "appearance" && _jsx(ThemeSettings, {}), tab === "notifications" && _jsx(NotificationSettings, {}), tab === "account" && _jsx(AccountSection, {})] }) })] }));
}
function AccountSection() {
    const qc = useQueryClient();
    const { data: user } = useUserServiceGetCurrentUser();
    const [password, setPassword] = useState("");
    const [confirm, setConfirm] = useState("");
    const [error, setError] = useState("");
    const updateUser = useUserServiceUpdateUser({
        mutation: {
            onSuccess: () => {
                void qc.invalidateQueries({ queryKey: getUserServiceGetCurrentUserQueryKey() });
                setPassword("");
                setConfirm("");
                setError("");
            },
        },
    });
    const handleSubmit = () => {
        if (!user?.id)
            return;
        if (password !== confirm) {
            setError("Passwords do not match");
            return;
        }
        if (password.length < 8) {
            setError("Password must be at least 8 characters");
            return;
        }
        setError("");
        updateUser.mutate({ id: user.id, data: { password } });
    };
    return (_jsxs("div", { className: "flex flex-col gap-4", children: [_jsx("h3", { className: "text-sm font-semibold text-[var(--text)]", children: "Change Password" }), _jsx(Input, { label: "New password", type: "password", value: password, onChange: (e) => setPassword(e.target.value), error: error }), _jsx(Input, { label: "Confirm password", type: "password", value: confirm, onChange: (e) => setConfirm(e.target.value) }), _jsx(Button, { variant: "primary", loading: updateUser.isPending, onClick: handleSubmit, children: "Update password" })] }));
}
