// Settings panel rendered in the host's `settings.user.appearance` slot (the
// user's profile → Appearance). Lets each user set, PER ACTIVE PROFILE, the
// home city + temperature unit the weather widget uses. Values are stored in
// the plugin's OWN per-profile preference store via the host bridge
// (window.evey.prefs) — nothing leaves the plugin's namespace, and the widget
// reads the same prefs. Zero runtime deps: vanilla DOM.

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { Units } from "../domain/model/weather";
import { translate } from "../i18n/labels";

export const CITY_KEY = "residence.city";
export const UNITS_KEY = "residence.units";

export class SettingsApp {
  private locale = "en";
  private saving = false;

  constructor(private bridge: HostBridge) {}

  start(): void {
    this.bridge.onReady((init) => void this.boot(init));
  }

  private t(key: string): string {
    return translate(key, this.locale);
  }

  private async boot(init: HostInit): Promise<void> {
    this.locale = init.locale;
    document.documentElement.style.background = "transparent";
    document.body.style.margin = "0";
    document.body.style.background = "transparent";

    // Current values: the user's own pref wins over the admin config default.
    const [c, u] = await Promise.all([
      this.bridge.getPref(CITY_KEY),
      this.bridge.getPref(UNITS_KEY),
    ]);
    const city = (c && c.trim()) || init.city;
    const units: Units = u === "fahrenheit" ? "fahrenheit" : "celsius";
    this.render(init.theme !== "light", city, units);
  }

  private render(dark: boolean, city: string, units: Units): void {
    const text = dark ? "#e9eaed" : "#1b1c1f";
    const muted = dark ? "#9aa0a6" : "#5f6368";
    const field = dark ? "rgba(255,255,255,0.06)" : "rgba(0,0,0,0.04)";
    const border = dark ? "rgba(255,255,255,0.16)" : "rgba(0,0,0,0.16)";
    const accent = "#e8a33d";

    const style = document.createElement("style");
    style.textContent = `
      .wxs { font-family: system-ui, -apple-system, sans-serif; color: ${text}; display: flex; flex-direction: column; gap: 12px; }
      .wxs h3 { margin: 0; font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
      .wxs p { margin: 0; font-size: 12px; color: ${muted}; }
      .wxs label { font-size: 12px; font-weight: 500; color: ${muted}; display: block; margin-bottom: 4px; }
      .wxs input, .wxs select {
        width: 100%; box-sizing: border-box; padding: 8px 10px; font-size: 14px;
        color: ${text}; background: ${field}; border: 1px solid ${border};
        border-radius: 8px; outline: none;
      }
      .wxs input:focus, .wxs select:focus { border-color: ${accent}; }
      .wxs option { color: #1b1c1f; }
      .wxs .row { max-width: 420px; }
      .wxs button {
        align-self: flex-start; padding: 8px 16px; font-size: 14px; font-weight: 600;
        color: #1b1c1f; background: ${accent}; border: none; border-radius: 8px; cursor: pointer;
      }
      .wxs button:disabled { opacity: 0.6; cursor: default; }
    `;
    document.head.appendChild(style);

    const root = document.createElement("div");
    root.className = "wxs";
    root.innerHTML = `
      <h3><span aria-hidden="true">🌤️</span>${esc(this.t("set_title"))}</h3>
      <p>${esc(this.t("set_desc"))}</p>
      <div class="row">
        <label for="wxs-city">${esc(this.t("set_city"))}</label>
        <input id="wxs-city" type="text" autocomplete="off" placeholder="${esc(this.t("set_city_ph"))}">
      </div>
      <div class="row">
        <label for="wxs-units">${esc(this.t("set_units"))}</label>
        <select id="wxs-units">
          <option value="celsius">${esc(this.t("set_celsius"))}</option>
          <option value="fahrenheit">${esc(this.t("set_fahrenheit"))}</option>
        </select>
      </div>
      <button id="wxs-save" type="button">${esc(this.t("set_save"))}</button>
    `;
    document.body.appendChild(root);

    const cityEl = root.querySelector<HTMLInputElement>("#wxs-city")!;
    const unitsEl = root.querySelector<HTMLSelectElement>("#wxs-units")!;
    const saveEl = root.querySelector<HTMLButtonElement>("#wxs-save")!;
    cityEl.value = city;
    unitsEl.value = units;
    saveEl.addEventListener("click", () => void this.save(cityEl.value, unitsEl.value, saveEl));

    this.fitHeight();
  }

  private async save(city: string, units: string, btn: HTMLButtonElement): Promise<void> {
    if (this.saving) return;
    this.saving = true;
    btn.disabled = true;
    try {
      const u: Units = units === "fahrenheit" ? "fahrenheit" : "celsius";
      await Promise.all([
        this.bridge.setPref(CITY_KEY, city.trim()),
        this.bridge.setPref(UNITS_KEY, u),
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
