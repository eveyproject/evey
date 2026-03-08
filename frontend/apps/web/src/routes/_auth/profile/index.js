import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useUserServiceGetCurrentUser, useUserServiceUpdateUser } from "@evey/api";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { useQueryClient } from "@tanstack/react-query";
import { getUserServiceGetCurrentUserQueryKey } from "@evey/api";
export const Route = createFileRoute("/_auth/profile/")({
    component: ProfilePage,
});
function ProfilePage() {
    const qc = useQueryClient();
    const { data: user, isLoading } = useUserServiceGetCurrentUser();
    const [editing, setEditing] = useState(false);
    const [username, setUsername] = useState("");
    const [email, setEmail] = useState("");
    const startEdit = (u) => {
        setUsername(u.username ?? "");
        setEmail(u.email ?? "");
        setEditing(true);
    };
    const updateUser = useUserServiceUpdateUser({
        mutation: {
            onSuccess: () => {
                void qc.invalidateQueries({ queryKey: getUserServiceGetCurrentUserQueryKey() });
                setEditing(false);
            },
        },
    });
    const handleSave = () => {
        if (!user?.id)
            return;
        updateUser.mutate({ id: user.id, data: { username, email } });
    };
    if (isLoading) {
        return (_jsx("div", { className: "px-6 py-8 text-sm text-[var(--text-muted)]", children: "Loading..." }));
    }
    return (_jsxs("div", { className: "px-6 py-8 max-w-2xl flex flex-col gap-6", children: [_jsx("h1", { className: "text-2xl font-bold text-[var(--text)]", children: "Profile" }), _jsx(Card, { children: _jsxs("div", { className: "p-6 flex flex-col gap-4", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsxs("div", { className: "flex items-center gap-4", children: [_jsx("div", { className: "w-14 h-14 rounded-full bg-[var(--accent)] flex items-center justify-center text-white text-xl font-bold", children: user?.username?.charAt(0).toUpperCase() ?? "U" }), _jsxs("div", { children: [_jsx("p", { className: "font-semibold text-[var(--text)]", children: user?.username ?? "—" }), _jsx("p", { className: "text-sm text-[var(--text-muted)]", children: user?.email ?? "—" }), user?.isAdmin && (_jsx("span", { className: "text-xs bg-[var(--accent)] text-white px-2 py-0.5 rounded-[var(--radius-sm)] mt-1 inline-block", children: "Admin" }))] })] }), !editing && (_jsx(Button, { variant: "secondary", onClick: () => user && startEdit(user), children: "Edit" }))] }), editing && (_jsxs("div", { className: "flex flex-col gap-3 pt-2 border-t border-[var(--border)]", children: [_jsx(Input, { label: "Username", value: username, onChange: (e) => setUsername(e.target.value) }), _jsx(Input, { label: "Email", value: email, onChange: (e) => setEmail(e.target.value) }), _jsxs("div", { className: "flex gap-2", children: [_jsx(Button, { variant: "primary", loading: updateUser.isPending, onClick: handleSave, children: "Save" }), _jsx(Button, { variant: "ghost", onClick: () => setEditing(false), children: "Cancel" })] })] }))] }) }), _jsxs("nav", { className: "flex flex-col gap-2", children: [_jsxs("a", { href: "/profile/settings", className: "flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--text)] hover:bg-[var(--border)] transition-colors", children: [_jsx("span", { children: "Settings" }), _jsx("span", { className: "text-[var(--text-muted)]", children: "\u203A" })] }), _jsxs("a", { href: "/profile/history", className: "flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--text)] hover:bg-[var(--border)] transition-colors", children: [_jsx("span", { children: "Watch History" }), _jsx("span", { className: "text-[var(--text-muted)]", children: "\u203A" })] }), _jsxs("a", { href: "/profile/notifications", className: "flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--text)] hover:bg-[var(--border)] transition-colors", children: [_jsx("span", { children: "Notifications" }), _jsx("span", { className: "text-[var(--text-muted)]", children: "\u203A" })] }), user?.isAdmin && (_jsxs("a", { href: "/admin", className: "flex items-center justify-between p-4 bg-[var(--surface-raised)] rounded-[var(--radius-md)] text-sm text-[var(--accent)] hover:bg-[var(--border)] transition-colors", children: [_jsx("span", { children: "Admin Panel" }), _jsx("span", { className: "text-[var(--text-muted)]", children: "\u203A" })] }))] })] }));
}
