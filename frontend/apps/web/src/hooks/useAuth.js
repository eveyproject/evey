import { useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
export function useAuth() {
    const accessToken = useAuthStore((s) => s.accessToken);
    const navigate = useNavigate();
    useEffect(() => {
        if (!accessToken) {
            void navigate({ to: "/login" });
        }
    }, [accessToken, navigate]);
    return { isAuthenticated: !!accessToken };
}
