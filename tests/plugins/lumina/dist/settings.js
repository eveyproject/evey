"use strict";(()=>{var T={aurore:{id:"aurore",anchors:[[.24,.16,.52],[.62,.18,.56],[.86,.56,.22],[.82,.36,.46]],vignette:[.3,.14,.1]},neon:{id:"neon",anchors:[[.05,.82,.92],[.92,.14,.72],[.52,.16,.94],[.14,.34,.96]],vignette:[.08,.02,.16]},braise:{id:"braise",anchors:[[.32,.05,.05],[.74,.14,.08],[.93,.46,.12],[.86,.62,.22]],vignette:[.16,.03,.02]},ocean:{id:"ocean",anchors:[[.02,.24,.34],[.05,.24,.62],[.1,.62,.78],[.18,.72,.58]],vignette:[.02,.1,.14]}},w=Object.keys(T),H="aurore";function $(r){return r==="aurore"||r==="neon"||r==="braise"||r==="ocean"}function d(r){return $(r)?r:H}var M={set_title:{en:"Ambient light",fr:"Lumi\xE8re ambiante",es:"Luz ambiental",de:"Umgebungslicht"},set_desc:{en:"Palette, intensity and speed for the ambient light-art background.",fr:"Palette, intensit\xE9 et vitesse du fond d'\xE9cran de lumi\xE8re ambiante.",es:"Paleta, intensidad y velocidad del fondo de luz ambiental.",de:"Palette, Intensit\xE4t und Geschwindigkeit des Umgebungslicht-Hintergrunds."},set_palette:{en:"Palette",fr:"Palette",es:"Paleta",de:"Palette"},set_intensity:{en:"Intensity",fr:"Intensit\xE9",es:"Intensidad",de:"Intensit\xE4t"},set_speed:{en:"Speed",fr:"Vitesse",es:"Velocidad",de:"Geschwindigkeit"},set_save:{en:"Save",fr:"Enregistrer",es:"Guardar",de:"Speichern"},set_saved:{en:"Lumina settings saved",fr:"R\xE9glages Lumina enregistr\xE9s",es:"Ajustes de Lumina guardados",de:"Lumina-Einstellungen gespeichert"},set_error:{en:"Couldn't save",fr:"Enregistrement impossible",es:"No se pudo guardar",de:"Speichern fehlgeschlagen"}};function I(r,t){var n;let s=M[r];if(!s)return r;let e=(t!=null?t:"en").slice(0,2).toLowerCase();return(n=s[e])!=null?n:s.en}var S="appearance.palette",L="appearance.intensity",N="appearance.speed",D={aurore:"Aurore",neon:"N\xE9on",braise:"Braise",ocean:"Oc\xE9an"},g=class{constructor(t){this.bridge=t;this.locale="en";this.saving=!1;this.booted=!1}start(){this.bridge.onReady(t=>void this.boot(t))}t(t){return I(t,this.locale)}async boot(t){if(this.booted)return;this.booted=!0,this.locale=t.locale,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent";let[s,e,n]=await Promise.all([this.bridge.getPref(S),this.bridge.getPref(L),this.bridge.getPref(N)]),o=s!==null?d(s):t.palette,l=e!==null?Number(e):NaN,f=Number.isFinite(l)?l:t.intensity,u=n!==null?Number(n):NaN,p=Number.isFinite(u)?u:t.speed;this.render(t.theme!=="light",o,f,p)}render(t,s,e,n){let o=t?"#e9eaed":"#1b1c1f",l=t?"#9aa0a6":"#5f6368",f=t?"rgba(255,255,255,0.06)":"rgba(0,0,0,0.04)",u=t?"rgba(255,255,255,0.16)":"rgba(0,0,0,0.16)",p="#b088ff",v=document.createElement("style");v.textContent=`
      .lums { font-family: system-ui, -apple-system, sans-serif; color: ${o}; display: flex; flex-direction: column; gap: 12px; }
      .lums h3 { margin: 0; font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
      .lums p { margin: 0; font-size: 12px; color: ${l}; }
      .lums label { font-size: 12px; font-weight: 500; color: ${l}; display: block; margin-bottom: 4px; }
      .lums select, .lums input[type="range"] {
        width: 100%; box-sizing: border-box; outline: none;
      }
      .lums select {
        padding: 8px 10px; font-size: 14px; color: ${o}; background: ${f};
        border: 1px solid ${u}; border-radius: 8px;
      }
      .lums select:focus { border-color: ${p}; }
      .lums option { color: #1b1c1f; }
      .lums .row { max-width: 420px; }
      .lums .rangeRow { display: flex; align-items: center; gap: 10px; }
      .lums .rangeVal { font-size: 12px; color: ${l}; min-width: 2.4em; text-align: right; }
      .lums button {
        align-self: flex-start; padding: 8px 16px; font-size: 14px; font-weight: 600;
        color: #16131f; background: ${p}; border: none; border-radius: 8px; cursor: pointer;
      }
      .lums button:disabled { opacity: 0.6; cursor: default; }
    `,document.head.appendChild(v);let i=document.createElement("div");i.className="lums",i.innerHTML=`
      <h3><span aria-hidden="true">\u2728</span>${a(this.t("set_title"))}</h3>
      <p>${a(this.t("set_desc"))}</p>
      <div class="row">
        <label for="lums-palette">${a(this.t("set_palette"))}</label>
        <select id="lums-palette">
          ${w.map(P=>`<option value="${P}">${a(D[P])}</option>`).join("")}
        </select>
      </div>
      <div class="row">
        <label for="lums-intensity">${a(this.t("set_intensity"))}</label>
        <div class="rangeRow">
          <input id="lums-intensity" type="range" min="0.2" max="1" step="0.05">
          <span id="lums-intensity-val" class="rangeVal"></span>
        </div>
      </div>
      <div class="row">
        <label for="lums-speed">${a(this.t("set_speed"))}</label>
        <div class="rangeRow">
          <input id="lums-speed" type="range" min="0.3" max="2" step="0.1">
          <span id="lums-speed-val" class="rangeVal"></span>
        </div>
      </div>
      <button id="lums-save" type="button">${a(this.t("set_save"))}</button>
    `,document.body.appendChild(i);let y=i.querySelector("#lums-palette"),c=i.querySelector("#lums-intensity"),h=i.querySelector("#lums-intensity-val"),m=i.querySelector("#lums-speed"),x=i.querySelector("#lums-speed-val"),E=i.querySelector("#lums-save");y.value=s,c.value=String(e),h.textContent=e.toFixed(2),m.value=String(n),x.textContent=`${n.toFixed(1)}x`,c.addEventListener("input",()=>{h.textContent=Number(c.value).toFixed(2)}),m.addEventListener("input",()=>{x.textContent=`${Number(m.value).toFixed(1)}x`}),E.addEventListener("click",()=>void this.save(y.value,Number(c.value),Number(m.value),E)),this.fitHeight()}async save(t,s,e,n){if(!this.saving){this.saving=!0,n.disabled=!0;try{await Promise.all([this.bridge.setPref(S,d(t)),this.bridge.setPref(L,String(s)),this.bridge.setPref(N,String(e))]),this.bridge.toast("success",this.t("set_saved"))}catch{this.bridge.toast("error",this.t("set_error"))}finally{this.saving=!1,n.disabled=!1}}}fitHeight(){let t=Math.max(120,Math.ceil(document.body.scrollHeight)+8);this.bridge.resize(t)}};function a(r){return r.replace(/[&<>"']/g,t=>t==="&"?"&amp;":t==="<"?"&lt;":t===">"?"&gt;":t==='"'?"&quot;":"&#39;")}var R=.7,z=1,B=.2,k=1,A=.3,F=2;function _(r,t,s,e){let n=typeof r=="number"?r:typeof r=="string"?Number(r):NaN;return Number.isFinite(n)?Math.min(e,Math.max(s,n)):t}var b=class{onReady(t){let s=(e,n,o)=>({palette:d(e==null?void 0:e.palette),intensity:_(e==null?void 0:e.intensity,R,B,k),speed:_(e==null?void 0:e.speed,z,A,F),locale:n!=null?n:"en",theme:o!=null?o:"dark"});typeof window!="undefined"&&window.evey?window.evey.onInit(e=>t(s(e==null?void 0:e.config,e==null?void 0:e.locale,e==null?void 0:e.theme))):t(s(void 0,navigator==null?void 0:navigator.language,void 0))}resize(t){var s;(s=window.evey)==null||s.resize(Math.round(t))}async getPref(t){var e;let s=(e=window.evey)==null?void 0:e.prefs;if(!(s!=null&&s.get))return null;try{let n=await s.get(t);return n==null?null:typeof n=="string"?n:n.found===!1?null:typeof n.value=="string"?n.value:null}catch{return null}}async setPref(t,s){var n;let e=(n=window.evey)==null?void 0:n.prefs;if(e!=null&&e.set)try{await e.set(t,s)}catch{}}toast(t,s){var e,n;(n=(e=window.evey)==null?void 0:e.toast)==null||n.call(e,t,s)}};new g(new b).start();})();
