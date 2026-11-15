import { useEffect } from "react";
import { getPlayerVolume, setPlayerVolume } from "@/hooks/usePlaybackPrefs";

// usePersistentVolume carries the player's volume from one title to the next.
// The player is remounted per title, so its <video> is a new element at full
// volume each time: the stored level is put back on it at mount, and every
// change (slider, wheel, keyboard, touch all write video.volume) is stored.
//
// Mute is left alone on purpose: the muted-autoplay fallback flips video.muted
// by itself, and a stored mute would start the next title silent.
export function usePersistentVolume(videoRef: React.RefObject<HTMLVideoElement | null>) {
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const stored = getPlayerVolume();
    if (video.volume !== stored) video.volume = stored;
    const onVolumeChange = () => setPlayerVolume(video.volume);
    video.addEventListener("volumechange", onVolumeChange);
    return () => video.removeEventListener("volumechange", onVolumeChange);
  }, [videoRef]);
}
