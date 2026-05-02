// Frame-rate cap + document.hidden skip.
// Wraps R3F useFrame with a 30-fps accumulator so heavy shaders
// don't drain battery on idle screens.

import { useRef } from "react";
import { useFrame } from "@react-three/fiber";
import type { RootState } from "@react-three/fiber";

const TARGET_INTERVAL = 1 / 30; // 30 fps in seconds

export function useFrameThrottle(
  cb: (state: RootState, delta: number) => void,
  renderPriority = 0
): void {
  const acc = useRef(0);
  useFrame((state, delta) => {
    if (document.hidden) return;
    acc.current += delta;
    if (acc.current < TARGET_INTERVAL) return;
    // Clamp delta sent to callback to avoid huge jumps after tab switch.
    const dt = Math.min(acc.current, TARGET_INTERVAL * 3);
    acc.current = 0;
    cb(state, dt);
  }, renderPriority);
}
