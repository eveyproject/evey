import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState, useEffect } from "react";
import { useNotificationServiceGetNotificationSettings, useNotificationServiceUpdateNotificationSettings, } from "@evey/api";
import { Button } from "@/components/ui/Button";
const LEVELS = [
    "NOTIFICATION_LEVEL_DISABLED",
    "NOTIFICATION_LEVEL_HIGHLY_RELEVANT",
    "NOTIFICATION_LEVEL_RELEVANT",
    "NOTIFICATION_LEVEL_ALL",
];
const LEVEL_LABELS = {
    NOTIFICATION_LEVEL_UNSPECIFIED: "Default",
    NOTIFICATION_LEVEL_DISABLED: "Disabled",
    NOTIFICATION_LEVEL_ALL: "All",
    NOTIFICATION_LEVEL_RELEVANT: "Relevant",
    NOTIFICATION_LEVEL_HIGHLY_RELEVANT: "Important only",
};
function LevelSelect({ label, value, onChange }) {
    return (_jsxs("div", { className: "flex items-center justify-between py-3 border-b border-[var(--border)] last:border-0", children: [_jsx("span", { className: "text-sm text-[var(--text)]", children: label }), _jsx("select", { value: value ?? "NOTIFICATION_LEVEL_UNSPECIFIED", onChange: (e) => onChange(e.target.value), className: "text-sm bg-[var(--surface-raised)] border border-[var(--border)] rounded-[var(--radius-sm)] px-2 py-1 text-[var(--text)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]", children: LEVELS.map((l) => (_jsx("option", { value: l, children: LEVEL_LABELS[l] }, l))) })] }));
}
export function NotificationSettings() {
    const { data: settings, isLoading } = useNotificationServiceGetNotificationSettings();
    const [movieLevel, setMovieLevel] = useState("NOTIFICATION_LEVEL_UNSPECIFIED");
    const [seriesLevel, setSeriesLevel] = useState("NOTIFICATION_LEVEL_UNSPECIFIED");
    const [musicLevel, setMusicLevel] = useState("NOTIFICATION_LEVEL_UNSPECIFIED");
    useEffect(() => {
        if (settings) {
            if (settings.movieLevel)
                setMovieLevel(settings.movieLevel);
            if (settings.seriesLevel)
                setSeriesLevel(settings.seriesLevel);
            if (settings.musicLevel)
                setMusicLevel(settings.musicLevel);
        }
    }, [settings]);
    const update = useNotificationServiceUpdateNotificationSettings({
        mutation: { onSuccess: () => { } },
    });
    const handleSave = () => {
        update.mutate({
            data: { movieLevel, seriesLevel, musicLevel },
        });
    };
    if (isLoading) {
        return _jsx("p", { className: "text-sm text-[var(--text-muted)]", children: "Loading..." });
    }
    return (_jsxs("div", { className: "flex flex-col gap-4", children: [_jsxs("div", { className: "bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4", children: [_jsx(LevelSelect, { label: "Movies", value: movieLevel, onChange: setMovieLevel }), _jsx(LevelSelect, { label: "Series", value: seriesLevel, onChange: setSeriesLevel }), _jsx(LevelSelect, { label: "Music", value: musicLevel, onChange: setMusicLevel })] }), _jsx("div", { children: _jsx(Button, { variant: "primary", loading: update.isPending, onClick: handleSave, children: "Save" }) })] }));
}
