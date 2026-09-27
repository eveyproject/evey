import { useEffect, useState } from "react";

interface UseOverlayTimeArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  transcodeOffsetRef: React.MutableRefObject<number>;
  knownDurationSecRef: React.MutableRefObject<number>;
}

export function useOverlayTime({
  videoRef,
  transcodeOffsetRef,
  knownDurationSecRef,
}: UseOverlayTimeArgs) {
  const [overlayTime, setOverlayTime] = useState(0);
  const [overlayDuration, setOverlayDuration] = useState(0);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    function onTime() {
      const v = videoRef.current;
      if (!v) return;
      const absTime = transcodeOffsetRef.current + v.currentTime;
      setOverlayTime(absTime);
      const d = knownDurationSecRef.current;
      if (d > 0) setOverlayDuration(d);
      else {
        const vd = v.duration;
        if (vd && isFinite(vd) && vd > 0) setOverlayDuration(transcodeOffsetRef.current + vd);
      }
    }
    video.addEventListener("timeupdate", onTime);
    return () => video.removeEventListener("timeupdate", onTime);
  }, [videoRef, transcodeOffsetRef, knownDurationSecRef]);

  return { overlayTime, overlayDuration };
}
