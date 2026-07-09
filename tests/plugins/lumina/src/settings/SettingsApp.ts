// Settings panel rendered in the host's `settings.user.appearance` slot (the
// user's profile -> Appearance). Lets each user set, PER ACTIVE PROFILE, the
// palette, intensity and speed of the ambient light background. Stored in
// the plugin's OWN per-profile preference store (window.evey.prefs) -
// nothing leaves the plugin's namespace, and the widget reads the same
// prefs. Zero runtime deps: vanilla DOM.

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { PresetId } from "../domain/model/scene";
import { PRESET_IDS, presetOrDefault } from "../domain/services/presets";
import { translate } from "../i18n/labels";

export const PALETTE_KEY = "appearance.palette";
export const INTENSITY_KEY = "appearance.intensity";
export const SPEED_KEY = "appearance.speed";

// Display labels for the preset select - intentionally not run through
// translate(): stylized names kept as-is across locales (see i18n/labels.ts).
const PALETTE_LABELS: Record<PresetId, string> = {
  aurore: "Aurore",
  neon: "Néon",
  braise: "Braise",
  ocean: "Océan",
};

export class SettingsApp {
  private locale = "en";
  private saving = false;
  // Guards against a host calling onInit more than once (see the same guard
  // in LuminaApp/weather-v2): without it, a second call re-appended a whole
  // duplicate form into document.body on top of the first.
  private booted = false;

  constructor(private bridge: HostBridge) {}

  start(): void {
    this.bridge.onReady((init) => void this.boot(init));
  }

  private t(key: string): string {
    return translate(key, this.locale);
  }

  private async boot(init: HostInit): Promise<void> {
    if (this.booted) return;
    this.booted = true;
    this.locale = init.locale;
    document.documentElement.style.background = "transparent";
    document.body.style.margin = "0";
    document.body.style.background = "transparent";

    // Current values: the user's own pref wins over the admin config default.
    const [p, i, s] = await Promise.all([
      this.bridge.getPref(PALETTE_KEY),
      this.bridge.getPref(INTENSITY_KEY),
      this.bridge.getPref(SPEED_KEY),
    ]);
    const palette = p !== null ? presetOrDefault(p) : init.palette;
    const iNum = i !== null ? Number(i) : NaN;
    const intensity = Number.isFinite(iNum) ? iNum : init.intensity;
    const sNum = s !== null ? Number(s) : NaN;
    const speed = Number.isFinite(sNum) ? sNum : init.speed;
    this.render(init.theme !== "light", palette, intensity, speed);
  }

  private render(dark: boolean, palette: PresetId, intensity: number, speed: number): void {
    const text = dark ? "#e9eaed" : "#1b1c1f";
    const muted = dark ? "#9aa0a6" : "#5f6368";
    const field = dark ? "rgba(255,255,255,0.06)" : "rgba(0,0,0,0.04)";
    const border = dark ? "rgba(255,255,255,0.16)" : "rgba(0,0,0,0.16)";
    const accent = "#b088ff";

    const style = document.createElement("style");
    style.textContent = `
      .lums { font-family: system-ui, -apple-system, sans-serif; color: ${text}; display: flex; flex-direction: column; gap: 12px; }
      .lums h3 { margin: 0; font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
      .lums p { margin: 0; font-size: 12px; color: ${muted}; }
      .lums label { font-size: 12px; font-weight: 500; color: ${muted}; display: block; margin-bottom: 4px; }
      .lums select, .lums input[type="range"] {
        width: 100%; box-sizing: border-box; outline: none;
      }
      .lums select {
        padding: 8px 10px; font-size: 14px; color: ${text}; background: ${field};
        border: 1px solid ${border}; border-radius: 8px;
      }
      .lums select:focus { border-color: ${accent}; }
      .lums option { color: #1b1c1f; }
      .lums .row { max-width: 420px; }
      .lums .rangeRow { display: flex; align-items: center; gap: 10px; }
      .lums .rangeVal { font-size: 12px; color: ${muted}; min-width: 2.4em; text-align: right; }
      .lums button {
        align-self: flex-start; padding: 8px 16px; font-size: 14px; font-weight: 600;
        color: #16131f; background: ${accent}; border: none; border-radius: 8px; cursor: pointer;
      }
      .lums button:disabled { opacity: 0.6; cursor: default; }
    `;
    document.head.appendChild(style);

    const root = document.createElement("div");
    root.className = "lums";
    root.innerHTML = `
      <h3><span aria-hidden="true">✨</span>${esc(this.t("set_title"))}</h3>
      <p>${esc(this.t("set_desc"))}</p>
      <div class="row">
        <label for="lums-palette">${esc(this.t("set_palette"))}</label>
        <select id="lums-palette">
          ${PRESET_IDS.map((id) => `<option value="${id}">${esc(PALETTE_LABELS[id])}</option>`).join("")}
        </select>
      </div>
      <div class="row">
        <label for="lums-intensity">${esc(this.t("set_intensity"))}</label>
        <div class="rangeRow">
          <input id="lums-intensity" type="range" min="0.2" max="1" step="0.05">
          <span id="lums-intensity-val" class="rangeVal"></span>
        </div>
      </div>
      <div class="row">
        <label for="lums-speed">${esc(this.t("set_speed"))}</label>
        <div class="rangeRow">
          <input id="lums-speed" type="range" min="0.3" max="2" step="0.1">
          <span id="lums-speed-val" class="rangeVal"></span>
        </div>
      </div>
      <button id="lums-save" type="button">${esc(this.t("set_save"))}</button>
    `;
    document.body.appendChild(root);

    const paletteEl = root.querySelector<HTMLSelectElement>("#lums-palette")!;
    const intensityEl = root.querySelector<HTMLInputElement>("#lums-intensity")!;
    const intensityValEl = root.querySelector<HTMLSpanElement>("#lums-intensity-val")!;
    const speedEl = root.querySelector<HTMLInputElement>("#lums-speed")!;
    const speedValEl = root.querySelector<HTMLSpanElement>("#lums-speed-val")!;
    const saveEl = root.querySelector<HTMLButtonElement>("#lums-save")!;

    paletteEl.value = palette;
    intensityEl.value = String(intensity);
    intensityValEl.textContent = intensity.toFixed(2);
    speedEl.value = String(speed);
    speedValEl.textContent = `${speed.toFixed(1)}x`;

    intensityEl.addEventListener("input", () => {
      intensityValEl.textContent = Number(intensityEl.value).toFixed(2);
    });
    speedEl.addEventListener("input", () => {
      speedValEl.textContent = `${Number(speedEl.value).toFixed(1)}x`;
    });
    saveEl.addEventListener(
      "click",
      () => void this.save(paletteEl.value, Number(intensityEl.value), Number(speedEl.value), saveEl),
    );

    this.fitHeight();
  }

  private async save(palette: string, intensity: number, speed: number, btn: HTMLButtonElement): Promise<void> {
    if (this.saving) return;
    this.saving = true;
    btn.disabled = true;
    try {
      await Promise.all([
        this.bridge.setPref(PALETTE_KEY, presetOrDefault(palette)),
        this.bridge.setPref(INTENSITY_KEY, String(intensity)),
        this.bridge.setPref(SPEED_KEY, String(speed)),
      ]);
      this.bridge.toast("success", this.t("set_saved"));
    } catch {
      this.bridge.toast("error", this.t("set_error"));
    } finally {
      this.saving = false;
      btn.disabled = false;
    }
  }

  private fitHeight(): void {
    const h = Math.max(120, Math.ceil(document.body.scrollHeight) + 8);
    this.bridge.resize(h);
  }
}

// Minimal HTML-escape for the small set of interpolated i18n strings.
function esc(s: string): string {
  return s.replace(/[&<>"']/g, (c) =>
    c === "&" ? "&amp;" : c === "<" ? "&lt;" : c === ">" ? "&gt;" : c === '"' ? "&quot;" : "&#39;",
  );
}
