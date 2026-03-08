import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute, isRedirect, redirect, useNavigate } from "@tanstack/react-router";
import { eveyFetch, useAdminServiceSetupWizard } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
export const Route = createFileRoute("/setup")({
    beforeLoad: async ({ context }) => {
        try {
            const info = await context.queryClient.fetchQuery({
                queryKey: ["system-info"],
                queryFn: () => eveyFetch({ url: "/api/v1/admin/system", method: "GET" }),
                staleTime: 30_000,
            });
            if (info.setupComplete)
                throw redirect({ to: "/login" });
        }
        catch (e) {
            if (isRedirect(e))
                throw e;
            // backend unreachable — show setup page
        }
    },
    component: SetupPage,
});
function SetupPage() {
    const navigate = useNavigate();
    const [form, setForm] = useState({
        adminUsername: "",
        adminPassword: "",
        adminEmail: "",
        serverName: "My Evey Server",
        baseUrl: window.location.origin,
    });
    const setup = useAdminServiceSetupWizard({
        mutation: {
            onSuccess: () => {
                void navigate({ to: "/login" });
            },
            onError: (err) => {
                if (err?.httpStatus === 409)
                    void navigate({ to: "/login" });
            },
        },
    });
    const update = (field) => (e) => setForm((prev) => ({ ...prev, [field]: e.target.value }));
    return (_jsx("div", { className: "min-h-screen flex items-center justify-center bg-[var(--bg)]", children: _jsxs(Card, { className: "w-full max-w-md", children: [_jsx("h1", { className: "text-2xl font-bold text-[var(--text)] mb-1", children: "Welcome to Evey" }), _jsx("p", { className: "text-[var(--text-muted)] text-sm mb-6", children: "Set up your server to get started" }), _jsxs("form", { onSubmit: (e) => {
                        e.preventDefault();
                        setup.mutate({
                            data: {
                                adminUsername: form.adminUsername,
                                adminPassword: form.adminPassword,
                                adminEmail: form.adminEmail,
                                serverName: form.serverName,
                                baseUrl: form.baseUrl,
                            },
                        });
                    }, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "Admin username", value: form.adminUsername, onChange: update("adminUsername"), required: true }), _jsx(Input, { label: "Admin password", type: "password", value: form.adminPassword, onChange: update("adminPassword"), required: true }), _jsx(Input, { label: "Admin email", type: "email", value: form.adminEmail, onChange: update("adminEmail"), required: true }), _jsx(Input, { label: "Server name", value: form.serverName, onChange: update("serverName"), required: true }), _jsx(Input, { label: "Base URL", value: form.baseUrl, onChange: update("baseUrl"), required: true }), setup.error && (_jsx("p", { className: "text-sm text-red-400", children: setup.error instanceof Error ? setup.error.message : "Setup failed" })), _jsx(Button, { type: "submit", loading: setup.isPending, className: "w-full mt-2", children: "Complete setup" })] })] }) }));
}
