import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { createFileRoute } from "@tanstack/react-router";
import { WatchHistory } from "@/features/user/WatchHistory";
export const Route = createFileRoute("/_auth/profile/history")({
    component: HistoryPage,
});
function HistoryPage() {
    return (_jsxs("div", { className: "px-6 py-8 max-w-2xl flex flex-col gap-6", children: [_jsx("h1", { className: "text-2xl font-bold text-[var(--text)]", children: "Watch History" }), _jsx(WatchHistory, {})] }));
}
