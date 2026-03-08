import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import { PlayerControls } from "./PlayerControls";
import { ProgressReporter } from "./ProgressReporter";
export function HLSPlayer({ streamInfo, sessionId, startPositionMs = 0, onEnded, audioStreams = [], subtitleStreams = [], title, onClose, }) {
    const videoRef = useRef(null);
    const [currentAudioIndex, setCurrentAudioIndex] = useState(audioStreams.find((s) => s.isDefault)?.index ?? audioStreams[0]?.index ?? 0);
    useEffect(() => {
        const video = videoRef.current;
        if (!video)
            return;
        let hlsInstance = null;
        if (streamInfo.transcodeUrl && Hls.isSupported()) {
            hlsInstance = new Hls();
            hlsInstance.loadSource(streamInfo.transcodeUrl);
            hlsInstance.attachMedia(video);
            hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
                video.currentTime = startPositionMs / 1000;
                void video.play();
            });
        }
        else if (streamInfo.directPlayUrl) {
            video.src = streamInfo.directPlayUrl;
            video.currentTime = startPositionMs / 1000;
            void video.play();
        }
        function handleEnded() {
            onEnded?.();
        }
        video.addEventListener("ended", handleEnded);
        return () => {
            video.removeEventListener("ended", handleEnded);
            if (hlsInstance) {
                hlsInstance.destroy();
                hlsInstance = null;
            }
        };
    }, [streamInfo, startPositionMs, onEnded]);
    function handleAudioChange(index) {
        setCurrentAudioIndex(index);
        const video = videoRef.current;
        if (!video)
            return;
        // audioTracks may not exist in all browsers
        // audioTracks is non-standard; cast to any to avoid TS DOM lib conflicts
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const tracks = video.audioTracks;
        if (!tracks)
            return;
        for (let i = 0; i < tracks.length; i++) {
            tracks[i].enabled = tracks[i].id === String(index);
        }
    }
    return (_jsxs("div", { className: "relative w-full h-full bg-black", children: [_jsx("video", { ref: videoRef, className: "w-full h-full", playsInline: true }), _jsx(ProgressReporter, { sessionId: sessionId, videoRef: videoRef }), _jsx(PlayerControls, { videoRef: videoRef, audioStreams: audioStreams, subtitleStreams: subtitleStreams, title: title, onClose: onClose, currentAudioIndex: currentAudioIndex, onAudioChange: handleAudioChange })] }));
}
