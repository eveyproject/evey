import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { useUserServiceLogin } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";
export const Route = createFileRoute("/login")({
    beforeLoad: () => {
        const { accessToken } = useAuthStore.getState();
        if (accessToken)
            throw redirect({ to: "/home" });
    },
    component: LoginPage,
});
function LoginPage() {
    const [username, setUsername] = useState("");
    const [password, setPassword] = useState("");
    const { setTokens } = useAuthStore();
    const navigate = useNavigate();
    const login = useUserServiceLogin({
        mutation: {
            onSuccess: (data) => {
                if (data.accessToken && data.refreshToken) {
                    setTokens(data.accessToken, data.refreshToken);
                    void navigate({ to: "/home" });
                }
            },
        },
    });
    return (_jsx("div", { className: "min-h-screen flex items-center justify-center bg-[var(--bg)]", children: _jsxs(Card, { className: "w-full max-w-sm", children: [_jsx("h1", { className: "text-2xl font-bold text-[var(--text)] mb-1 text-center", children: "Evey" }), _jsx("p", { className: "text-[var(--text-muted)] text-sm text-center mb-6", children: "Sign in to your account" }), _jsxs("form", { onSubmit: (e) => {
                        e.preventDefault();
                        login.mutate({ data: { username, password } });
                    }, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "Username", type: "text", value: username, onChange: (e) => setUsername(e.target.value), required: true, autoFocus: true }), _jsx(Input, { label: "Password", type: "password", value: password, onChange: (e) => setPassword(e.target.value), required: true }), login.error && (_jsx("p", { className: "text-sm text-red-400 text-center", children: login.error instanceof Error ? login.error.message : "Login failed" })), _jsx(Button, { type: "submit", loading: login.isPending, className: "w-full mt-2", children: "Sign in" })] })] }) }));
}
