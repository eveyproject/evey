// Settings panel rendered in the host's `settings.user.appearance` slot (the
// user's profile -> Appearance). Lets each user set, PER ACTIVE PROFILE, the
// city the sky widget uses to compute their local sky. Stored in the
// plugin's OWN per-profile preference store (window.evey.prefs) - nothing
// leaves the plugin's namespace. Zero runtime deps: vanilla DOM.

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import { translate } from "../i18n/labels";

export const CITY_KEY = "residence.city";

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

    const c = await this.bridge.getPref(CITY_KEY);
    const city = (c && c.trim()) || init.city;
    this.render(init.theme !== "light", city);
  }

  private render(dark: boolean, city: string): void {
    const text = dark ? "#e9eaed" : "#1b1c1f";
    const muted = dark ? "#9aa0a6" : "#5f6368";
    const field = dark ? "rgba(255,255,255,0.06)" : "rgba(0,0,0,0.04)";
    const border = dark ? "rgba(255,255,255,0.16)" : "rgba(0,0,0,0.16)";
    const accent = "#7c9cff";

    const style = document.createElement("style");
    style.textContent = `
      .coss { font-family: system-ui, -apple-system, sans-serif; color: ${text}; display: flex; flex-direction: column; gap: 12px; }
      .coss h3 { margin: 0; font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
      .coss p { margin: 0; font-size: 12px; color: ${muted}; }
      .coss label { font-size: 12px; font-weight: 500; color: ${muted}; display: block; margin-bottom: 4px; }
      .coss input {
        width: 100%; box-sizing: border-box; padding: 8px 10px; font-size: 14px;
        color: ${text}; background: ${field}; border: 1px solid ${border};
        border-radius: 8px; outline: none;
      }
      .coss input:focus { border-color: ${accent}; }
      .coss .row { max-width: 420px; }
      .coss button {
        align-self: flex-start; padding: 8px 16px; font-size: 14px; font-weight: 600;
        color: #0d1220; background: ${accent}; border: none; border-radius: 8px; cursor: pointer;
      }
      .coss button:disabled { opacity: 0.6; cursor: default; }
    `;
    document.head.appendChild(style);

    const root = document.createElement("div");
    root.className = "coss";
    root.innerHTML = `
      <h3><span aria-hidden="true">✨</span>${esc(this.t("set_title"))}</h3>
      <p>${esc(this.t("set_desc"))}</p>
      <div class="row">
        <label for="coss-city">${esc(this.t("set_city"))}</label>
        <input id="coss-city" type="text" autocomplete="off" placeholder="${esc(this.t("set_city_ph"))}">
      </div>
      <button id="coss-save" type="button">${esc(this.t("set_save"))}</button>
    `;
    document.body.appendChild(root);

    const cityEl = root.querySelector<HTMLInputElement>("#coss-city")!;
    const saveEl = root.querySelector<HTMLButtonElement>("#coss-save")!;
    cityEl.value = city;
    saveEl.addEventListener("click", () => void this.save(cityEl.value, saveEl));

    this.fitHeight();
  }

  private async save(city: string, btn: HTMLButtonElement): Promise<void> {
    if (this.saving) return;
    this.saving = true;
    btn.disabled = true;
    try {
      await this.bridge.setPref(CITY_KEY, city.trim());
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

function esc(s: string): string {
  return s.replace(/[&<>"']/g, (c) =>
    c === "&" ? "&amp;" : c === "<" ? "&lt;" : c === ">" ? "&gt;" : c === '"' ? "&quot;" : "&#39;",
  );
}
