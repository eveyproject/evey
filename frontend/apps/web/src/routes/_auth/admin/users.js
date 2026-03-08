import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceListUsers, useUserServiceCreateUser, useUserServiceDeleteUser, useUserServiceDisableUser, useUserServiceListInviteLinks, useUserServiceCreateInviteLink, useUserServiceRevokeInviteLink, } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
export const Route = createFileRoute("/_auth/admin/users")({
    component: UsersPage,
});
const USERS_QUERY_KEY = ["/api/v1/admin/users"];
const INVITES_QUERY_KEY = ["/api/v1/invites"];
function CreateUserForm({ onSuccess }) {
    const [username, setUsername] = useState("");
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [isAdmin, setIsAdmin] = useState(false);
    const createUser = useUserServiceCreateUser({ mutation: { onSuccess } });
    function handleSubmit(e) {
        e.preventDefault();
        createUser.mutate({ data: { username, email, password, isAdmin } });
    }
    return (_jsxs("form", { onSubmit: handleSubmit, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "Username", value: username, onChange: (e) => setUsername(e.target.value), required: true }), _jsx(Input, { label: "Email", type: "email", value: email, onChange: (e) => setEmail(e.target.value) }), _jsx(Input, { label: "Password", type: "password", value: password, onChange: (e) => setPassword(e.target.value), required: true }), _jsxs("label", { className: "flex items-center gap-2 text-sm cursor-pointer", style: { color: "var(--text)" }, children: [_jsx("input", { type: "checkbox", checked: isAdmin, onChange: (e) => setIsAdmin(e.target.checked) }), "Admin"] }), _jsx("div", { className: "flex justify-end pt-2", children: _jsx(Button, { type: "submit", loading: createUser.isPending, disabled: !username || !password, children: "Create User" }) })] }));
}
function UserRow({ user }) {
    const queryClient = useQueryClient();
    const deleteUser = useUserServiceDeleteUser({
        mutation: { onSuccess: () => queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY }) },
    });
    const disableUser = useUserServiceDisableUser({
        mutation: { onSuccess: () => queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY }) },
    });
    return (_jsxs("tr", { style: { borderBottom: "1px solid var(--border)" }, children: [_jsx("td", { className: "px-4 py-3 font-medium", style: { color: "var(--text)" }, children: user.username }), _jsx("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: user.email ?? "—" }), _jsx("td", { className: "px-4 py-3", children: user.isAdmin ? _jsx(Badge, { variant: "warning", children: "Admin" }) : _jsx(Badge, { children: "User" }) }), _jsx("td", { className: "px-4 py-3", children: user.enabled !== false ? (_jsx(Badge, { variant: "success", children: "Enabled" })) : (_jsx(Badge, { variant: "error", children: "Disabled" })) }), _jsx("td", { className: "px-4 py-3 whitespace-nowrap", children: _jsxs("div", { className: "flex gap-2", children: [_jsx(Button, { variant: "secondary", loading: disableUser.isPending, onClick: () => disableUser.mutate({ id: user.id, params: { disabled: user.enabled !== false } }), children: user.enabled !== false ? "Disable" : "Enable" }), _jsx(Button, { variant: "danger", loading: deleteUser.isPending, onClick: () => {
                                if (confirm(`Delete user "${user.username}"?`)) {
                                    deleteUser.mutate({ id: user.id });
                                }
                            }, children: "Delete" })] }) })] }));
}
function InviteLinkRow({ link }) {
    const queryClient = useQueryClient();
    const revoke = useUserServiceRevokeInviteLink({
        mutation: { onSuccess: () => queryClient.invalidateQueries({ queryKey: INVITES_QUERY_KEY }) },
    });
    const inviteUrl = link.code ? `${window.location.origin}/register?invite=${link.code}` : "—";
    function copyLink() {
        if (link.code)
            navigator.clipboard.writeText(inviteUrl);
    }
    return (_jsxs("tr", { style: { borderBottom: "1px solid var(--border)" }, children: [_jsx("td", { className: "px-4 py-3 font-mono text-sm", style: { color: "var(--text)" }, children: link.code ?? "—" }), _jsxs("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: [link.timesUsed ?? 0, " / ", link.maxUses ?? "∞"] }), _jsx("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: link.expiresAt ? new Date(link.expiresAt).toLocaleString() : "Never" }), _jsx("td", { className: "px-4 py-3 whitespace-nowrap", children: _jsxs("div", { className: "flex gap-2", children: [_jsx(Button, { variant: "secondary", onClick: copyLink, children: "Copy Link" }), _jsx(Button, { variant: "danger", loading: revoke.isPending, onClick: () => {
                                if (confirm("Revoke this invite link?")) {
                                    revoke.mutate({ id: link.id });
                                }
                            }, children: "Revoke" })] }) })] }));
}
function CreateInviteForm({ onSuccess }) {
    const [maxUses, setMaxUses] = useState("");
    const [expiresAt, setExpiresAt] = useState("");
    const createInvite = useUserServiceCreateInviteLink({ mutation: { onSuccess } });
    function handleSubmit(e) {
        e.preventDefault();
        createInvite.mutate({
            data: {
                maxUses: maxUses ? parseInt(maxUses) : undefined,
                expiresAt: expiresAt || undefined,
            },
        });
    }
    return (_jsxs("form", { onSubmit: handleSubmit, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "Max uses (empty = unlimited)", type: "number", min: 1, value: maxUses, onChange: (e) => setMaxUses(e.target.value) }), _jsx(Input, { label: "Expires at (optional)", type: "datetime-local", value: expiresAt, onChange: (e) => setExpiresAt(e.target.value) }), _jsx("div", { className: "flex justify-end pt-2", children: _jsx(Button, { type: "submit", loading: createInvite.isPending, children: "Create Invite Link" }) })] }));
}
function UsersPage() {
    const [showUserModal, setShowUserModal] = useState(false);
    const [showInviteModal, setShowInviteModal] = useState(false);
    const queryClient = useQueryClient();
    const { data, isLoading } = useUserServiceListUsers();
    const { data: inviteData, isLoading: inviteLoading } = useUserServiceListInviteLinks();
    const users = data?.users ?? [];
    const invites = inviteData?.inviteLinks ?? [];
    function handleCreateSuccess() {
        queryClient.invalidateQueries({ queryKey: USERS_QUERY_KEY });
        setShowUserModal(false);
    }
    function handleInviteSuccess() {
        queryClient.invalidateQueries({ queryKey: INVITES_QUERY_KEY });
        setShowInviteModal(false);
    }
    return (_jsxs("div", { className: "flex flex-col gap-8", children: [_jsxs("section", { className: "flex flex-col gap-4", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: "Users" }), _jsx(Button, { onClick: () => setShowUserModal(true), children: "Create User" })] }), isLoading ? (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })) : (_jsx("div", { className: "overflow-x-auto rounded-[var(--radius-lg)]", style: { border: "1px solid var(--border)" }, children: _jsxs("table", { className: "w-full text-sm", children: [_jsx("thead", { children: _jsx("tr", { style: { backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }, children: ["Username", "Email", "Role", "Status", "Actions"].map((h) => (_jsx("th", { className: "px-4 py-2 text-left font-medium", style: { color: "var(--text-muted)" }, children: h }, h))) }) }), _jsx("tbody", { children: users.length === 0 ? (_jsx("tr", { children: _jsx("td", { colSpan: 5, className: "px-4 py-8 text-center", style: { color: "var(--text-muted)" }, children: "No users found." }) })) : (users.map((u) => _jsx(UserRow, { user: u }, u.id))) })] }) }))] }), _jsxs("section", { className: "flex flex-col gap-4", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsx("h2", { className: "text-xl font-semibold", style: { color: "var(--text)" }, children: "Invite Links" }), _jsx(Button, { variant: "secondary", onClick: () => setShowInviteModal(true), children: "Create Invite Link" })] }), inviteLoading ? (_jsx("div", { className: "flex justify-center py-8", children: _jsx(Spinner, { size: "md" }) })) : (_jsx("div", { className: "overflow-x-auto rounded-[var(--radius-lg)]", style: { border: "1px solid var(--border)" }, children: _jsxs("table", { className: "w-full text-sm", children: [_jsx("thead", { children: _jsx("tr", { style: { backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }, children: ["Code", "Uses", "Expires", "Actions"].map((h) => (_jsx("th", { className: "px-4 py-2 text-left font-medium", style: { color: "var(--text-muted)" }, children: h }, h))) }) }), _jsx("tbody", { children: invites.length === 0 ? (_jsx("tr", { children: _jsx("td", { colSpan: 4, className: "px-4 py-8 text-center", style: { color: "var(--text-muted)" }, children: "No invite links." }) })) : (invites.map((link) => _jsx(InviteLinkRow, { link: link }, link.id))) })] }) }))] }), _jsx(Modal, { open: showUserModal, onClose: () => setShowUserModal(false), title: "Create User", children: _jsx(CreateUserForm, { onSuccess: handleCreateSuccess }) }), _jsx(Modal, { open: showInviteModal, onClose: () => setShowInviteModal(false), title: "Create Invite Link", children: _jsx(CreateInviteForm, { onSuccess: handleInviteSuccess }) })] }));
}
