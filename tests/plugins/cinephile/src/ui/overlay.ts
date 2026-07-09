// Vanilla-DOM widget shell: the Canvas2D galaxy host, a "your profile" stat
// card, a hover tooltip, the decade timeline strip and the empty-library
// message. Responsive sizing follows the weather-v2/cosmos/marquee pattern:
// CSS clamp() on container-query units, falling back to vw.

import type { TimelineBar } from "../domain/services/timeline";

export interface ProfileRow {
  label: string;
  value: string;
}

export interface EdgeLabels {
  start: string;
  end: string;
}

export interface OverlayHandle {
  canvasHost: HTMLElement;
  setTooltip(text: string | null): void;
  setProfile(rows: ProfileRow[] | null, heading: string): void;
  setTimeline(bars: TimelineBar[], edgeLabels: EdgeLabels | null): void;
  setEmptyMessage(text: string | null): void;
  dispose(): void;
}

const BASE_CSS = `
.cp-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: radial-gradient(120% 140% at 50% 0%, #201642 0%, #120b26 45%, #05040d 100%);
  display: flex;
  flex-direction: column;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
}
.cp-scene { position: relative; flex: 1 1 auto; min-height: 0; }
.cp-tooltip {
  position: absolute;
  left: 50%;
  top: clamp(8px, 4cqi, 16px);
  transform: translateX(-50%);
  max-width: 84%;
  padding: clamp(4px, 1.4cqi, 8px) clamp(10px, 3cqi, 16px);
  border-radius: 999px;
  background: rgba(12, 8, 26, 0.72);
  border: 1px solid rgba(200, 180, 255, 0.25);
  backdrop-filter: blur(6px);
  color: #f3edff;
  font-size: clamp(11px, 3cqi, 14px);
  letter-spacing: 0.01em;
  text-align: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
  opacity: 0;
  transition: opacity 140ms ease;
  z-index: 3;
}
.cp-tooltip.visible { opacity: 1; }
.cp-profile {
  position: absolute;
  left: clamp(8px, 3cqi, 16px);
  top: clamp(8px, 3cqi, 16px);
  padding: clamp(8px, 2.4cqi, 14px);
  border-radius: 12px;
  background: rgba(14, 9, 30, 0.6);
  border: 1px solid rgba(180, 150, 255, 0.2);
  backdrop-filter: blur(6px);
  color: #ece5ff;
  font-size: clamp(10px, 2.6cqi, 12px);
  max-width: 46%;
  z-index: 3;
  opacity: 0;
  transition: opacity 200ms ease;
}
.cp-profile.visible { opacity: 1; }
.cp-profile-heading {
  font-weight: 600;
  letter-spacing: 0.02em;
  margin-bottom: clamp(4px, 1.4cqi, 8px);
  color: #d9c8ff;
  font-size: clamp(11px, 2.8cqi, 13px);
}
.cp-profile-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 1px 0;
  color: rgba(236, 229, 255, 0.85);
}
.cp-profile-row .cp-profile-value {
  color: #ffffff;
  font-weight: 600;
}
.cp-timeline {
  flex: 0 0 auto;
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: clamp(30px, 10cqi, 56px);
  padding: 4px clamp(8px, 3cqi, 16px) clamp(14px, 3.6cqi, 20px);
  z-index: 2;
}
.cp-timeline-bar {
  flex: 1 1 0;
  min-width: 2px;
  min-height: 2px;
  border-radius: 2px 2px 0 0;
  background: linear-gradient(180deg, #b79bff 0%, #6f4fd6 100%);
  opacity: 0.85;
  transition: height 400ms ease;
}
.cp-timeline-edge {
  position: absolute;
  bottom: 2px;
  font-size: clamp(8px, 2cqi, 10px);
  color: rgba(220, 210, 255, 0.55);
  letter-spacing: 0.02em;
}
.cp-timeline-edge.start { left: clamp(8px, 3cqi, 16px); }
.cp-timeline-edge.end { right: clamp(8px, 3cqi, 16px); }
.cp-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 10%;
  text-align: center;
  color: rgba(236, 229, 255, 0.75);
  font-size: clamp(11px, 3.2cqi, 14px);
  pointer-events: none;
  z-index: 4;
}
`;

const FALLBACK_CSS = BASE_CSS.replace(/cqi/g, "vw");

export function createOverlay(parent: HTMLElement): OverlayHandle {
  const style = document.createElement("style");
  const supportsCq = window.CSS?.supports?.("width: 1cqi") === true;
  style.textContent = supportsCq
    ? `.cp-root { container-type: inline-size; }\n${BASE_CSS}`
    : FALLBACK_CSS;
  document.head.appendChild(style);

  const root = document.createElement("div");
  root.className = "cp-root";

  const scene = document.createElement("div");
  scene.className = "cp-scene";
  root.appendChild(scene);

  const tooltip = document.createElement("div");
  tooltip.className = "cp-tooltip";
  scene.appendChild(tooltip);

  const profile = document.createElement("div");
  profile.className = "cp-profile";
  const profileHeading = document.createElement("div");
  profileHeading.className = "cp-profile-heading";
  profile.appendChild(profileHeading);
  const profileRows = document.createElement("div");
  profile.appendChild(profileRows);
  scene.appendChild(profile);

  const timeline = document.createElement("div");
  timeline.className = "cp-timeline";
  root.appendChild(timeline);

  const empty = document.createElement("div");
  empty.className = "cp-empty";
  empty.style.display = "none";
  root.appendChild(empty);

  parent.appendChild(root);

  return {
    canvasHost: scene,
    setTooltip(text: string | null) {
      if (!text) {
        tooltip.classList.remove("visible");
        return;
      }
      tooltip.textContent = text;
      tooltip.classList.add("visible");
    },
    setProfile(rows: ProfileRow[] | null, heading: string) {
      if (!rows || rows.length === 0) {
        profile.classList.remove("visible");
        return;
      }
      profileHeading.textContent = heading;
      profileRows.replaceChildren();
      for (const row of rows) {
        const rowEl = document.createElement("div");
        rowEl.className = "cp-profile-row";
        const label = document.createElement("span");
        label.textContent = row.label;
        const value = document.createElement("span");
        value.className = "cp-profile-value";
        value.textContent = row.value;
        rowEl.appendChild(label);
        rowEl.appendChild(value);
        profileRows.appendChild(rowEl);
      }
      profile.classList.add("visible");
    },
    setTimeline(bars: TimelineBar[], edgeLabels: EdgeLabels | null) {
      timeline.replaceChildren();
      for (const bar of bars) {
        const el = document.createElement("div");
        el.className = "cp-timeline-bar";
        el.style.height = `${Math.max(4, bar.heightFraction * 100)}%`;
        timeline.appendChild(el);
      }
      if (edgeLabels) {
        const start = document.createElement("div");
        start.className = "cp-timeline-edge start";
        start.textContent = edgeLabels.start;
        timeline.appendChild(start);
        const end = document.createElement("div");
        end.className = "cp-timeline-edge end";
        end.textContent = edgeLabels.end;
        timeline.appendChild(end);
      }
    },
    setEmptyMessage(text: string | null) {
      empty.textContent = text ?? "";
      empty.style.display = text ? "flex" : "none";
    },
    dispose() {
      root.remove();
      style.remove();
    },
  };
}
