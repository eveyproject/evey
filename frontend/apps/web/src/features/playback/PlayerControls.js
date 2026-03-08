import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useCallback, useEffect, useRef, useState } from "react";
import { AudioTrackSelector } from "./AudioTrackSelector";
import { SubtitleSelector } from "./SubtitleSelector";
function formatTime(seconds) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    if (h > 0)
        return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
    return `${m}:${String(s).padStart(2, "0")}`;
}
export function PlayerControls({ videoRef, audioStreams, subtitleStreams, title, onClose, currentAudioIndex, onAudioChange, }) {
    const [visible, setVisible] = useState(true);
    const [playing, setPlaying] = useState(false);
    const [currentTime, setCurrentTime] = useState(0);
    const [duration, setDuration] = useState(0);
    const [volume, setVolume] = useState(1);
    const [fullscreen, setFullscreen] = useState(false);
    const hideTimer = useRef(null);
    const resetHideTimer = useCallback(() => {
        setVisible(true);
        if (hideTimer.current)
            clearTimeout(hideTimer.current);
        hideTimer.current = setTimeout(() => setVisible(false), 3000);
    }, []);
    useEffect(() => {
        resetHideTimer();
        return () => {
            if (hideTimer.current)
                clearTimeout(hideTimer.current);
        };
    }, [resetHideTimer]);
    useEffect(() => {
        const video = videoRef.current;
        if (!video)
            return;
        function onPlay() { setPlaying(true); }
        function onPause() { setPlaying(false); }
        function onTimeUpdate() { setCurrentTime(video.currentTime); }
        function onDurationChange() { setDuration(video.duration || 0); }
        function onVolumeChange() { setVolume(video.volume); }
        function onFullscreenChange() { setFullscreen(!!document.fullscreenElement); }
        video.addEventListener("play", onPlay);
        video.addEventListener("pause", onPause);
        video.addEventListener("timeupdate", onTimeUpdate);
        video.addEventListener("durationchange", onDurationChange);
        video.addEventListener("volumechange", onVolumeChange);
        document.addEventListener("fullscreenchange", onFullscreenChange);
        setPlaying(!video.paused);
        setCurrentTime(video.currentTime);
        setDuration(video.duration || 0);
        setVolume(video.volume);
        return () => {
            video.removeEventListener("play", onPlay);
            video.removeEventListener("pause", onPause);
            video.removeEventListener("timeupdate", onTimeUpdate);
            video.removeEventListener("durationchange", onDurationChange);
            video.removeEventListener("volumechange", onVolumeChange);
            document.removeEventListener("fullscreenchange", onFullscreenChange);
        };
    }, [videoRef]);
    function togglePlay() {
        const video = videoRef.current;
        if (!video)
            return;
        if (video.paused)
            void video.play();
        else
            video.pause();
    }
    function handleSeek(e) {
        const video = videoRef.current;
        if (!video)
            return;
        video.currentTime = Number(e.target.value);
    }
    function handleVolume(e) {
        const video = videoRef.current;
        if (!video)
            return;
        video.volume = Number(e.target.value);
    }
    function toggleFullscreen() {
        const video = videoRef.current;
        if (!video)
            return;
        const container = video.parentElement;
        if (!container)
            return;
        if (!document.fullscreenElement) {
            void container.requestFullscreen();
        }
        else {
            void document.exitFullscreen();
        }
    }
    return (_jsxs("div", { className: "absolute inset-0", onMouseMove: resetHideTimer, onClick: resetHideTimer, style: { cursor: visible ? "default" : "none" }, children: [_jsxs("div", { className: `absolute top-0 left-0 right-0 flex items-center gap-3 px-4 py-3 transition-opacity duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`, style: { background: "linear-gradient(to bottom, rgba(0,0,0,0.7), transparent)" }, children: [_jsx("button", { onClick: onClose, "aria-label": "Back", className: "text-white hover:text-[var(--accent)] transition-colors", children: _jsx("svg", { width: "24", height: "24", viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: "2", children: _jsx("path", { d: "M19 12H5M12 5l-7 7 7 7" }) }) }), title && _jsx("span", { className: "text-white text-sm font-medium truncate", children: title })] }), _jsxs("div", { className: `absolute bottom-0 left-0 right-0 px-4 pb-4 pt-8 flex flex-col gap-2 transition-opacity duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`, style: { background: "linear-gradient(to top, rgba(0,0,0,0.8), transparent)" }, children: [_jsx("input", { type: "range", min: 0, max: duration || 0, step: 0.5, value: currentTime, onChange: handleSeek, className: "w-full accent-[var(--accent)] cursor-pointer", "aria-label": "Seek" }), _jsxs("div", { className: "flex items-center gap-3", children: [_jsx("button", { onClick: togglePlay, "aria-label": playing ? "Pause" : "Play", className: "text-white hover:text-[var(--accent)] transition-colors", children: playing ? (_jsxs("svg", { width: "24", height: "24", viewBox: "0 0 24 24", fill: "currentColor", children: [_jsx("rect", { x: "6", y: "4", width: "4", height: "16" }), _jsx("rect", { x: "14", y: "4", width: "4", height: "16" })] })) : (_jsx("svg", { width: "24", height: "24", viewBox: "0 0 24 24", fill: "currentColor", children: _jsx("path", { d: "M5 3l14 9-14 9V3z" }) })) }), _jsxs("span", { className: "text-white text-xs tabular-nums", children: [formatTime(currentTime), " / ", formatTime(duration)] }), _jsxs("div", { className: "flex items-center gap-1", children: [_jsx("svg", { width: "18", height: "18", viewBox: "0 0 24 24", fill: "currentColor", className: "text-white shrink-0", children: _jsx("path", { d: "M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z" }) }), _jsx("input", { type: "range", min: 0, max: 1, step: 0.05, value: volume, onChange: handleVolume, className: "w-20 accent-[var(--accent)] cursor-pointer", "aria-label": "Volume" })] }), _jsx("div", { className: "flex-1" }), _jsx(SubtitleSelector, { streams: subtitleStreams, videoRef: videoRef }), _jsx(AudioTrackSelector, { streams: audioStreams, currentIndex: currentAudioIndex, onChange: onAudioChange }), _jsx("button", { onClick: toggleFullscreen, "aria-label": fullscreen ? "Exit fullscreen" : "Fullscreen", className: "text-white hover:text-[var(--accent)] transition-colors", children: fullscreen ? (_jsx("svg", { width: "20", height: "20", viewBox: "0 0 24 24", fill: "currentColor", children: _jsx("path", { d: "M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z" }) })) : (_jsx("svg", { width: "20", height: "20", viewBox: "0 0 24 24", fill: "currentColor", children: _jsx("path", { d: "M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z" }) })) })] })] })] }));
}
