import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useRouterState } from "@tanstack/react-router";
import { Sidebar } from "./Sidebar";
import { TopBar } from "./TopBar";
export function AuthLayout({ children }) {
    const { location } = useRouterState();
    const isHome = location.pathname === "/home";
    return (_jsxs("div", { className: "flex h-screen overflow-hidden", children: [_jsx(Sidebar, {}), _jsxs("div", { className: "flex flex-col flex-1 overflow-hidden", children: [_jsx(TopBar, {}), _jsx("main", { className: `flex-1 overflow-auto${isHome ? "" : " p-6"}`, children: children })] })] }));
}
