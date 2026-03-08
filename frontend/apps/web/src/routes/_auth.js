import { jsx as _jsx } from "react/jsx-runtime";
import { createFileRoute, Outlet, redirect } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { AuthLayout } from "@/components/layout/AuthLayout";
export const Route = createFileRoute("/_auth")({
    beforeLoad: () => {
        const { accessToken } = useAuthStore.getState();
        if (!accessToken)
            throw redirect({ to: "/login" });
    },
    component: () => (_jsx(AuthLayout, { children: _jsx(Outlet, {}) })),
});
