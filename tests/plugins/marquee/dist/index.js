"use strict";(()=>{function O(s){let t=Math.max(0,Math.floor(s.count)),e=Math.max(1,Math.floor(s.rows)),i=t===0?1:Math.max(1,Math.ceil(t/e)),r=i*s.colSpacing,o=[];for(let n=0;n<t;n++){let a=Math.floor(n/i),l=n%i,p=a%2===1?s.colSpacing*s.rowStaggerFraction:0,h=(l-(i-1)/2)*s.colSpacing+p,u=a*s.rowRise,d=e>1?a/(e-1):0,m=Math.pow(s.rowScaleFalloff,a);o.push({index:n,row:a,col:l,x:h,y:u,scale:m,z:d})}return{slots:o,rows:e,colsPerRow:i,period:r>0?r:s.colSpacing}}function P(s){let t=s>>>0;return function(){t|=0,t=t+1831565813|0;let i=Math.imul(t^t>>>15,1|t);return i=i+Math.imul(i^i>>>7,61|i)^i,((i^i>>>14)>>>0)/4294967296}}function F(s){let t=2166136261;for(let e=0;e<s.length;e++)t^=s.charCodeAt(e),t=Math.imul(t,16777619);return t>>>0}function M(s,t){for(let e=s.length-1;e>0;e--){let i=Math.floor(t()*(e+1)),r=s[e];s[e]=s[i],s[i]=r}return s}function D(s,t,e){let i=Math.max(0,Math.floor(t));if(i===0)return[];let r=new Set,o=[];for(let u of s)!u||!u.id||!u.posterUrl||r.has(u.id)||(r.add(u.id),o.push(u));let n=P(e);if(o.length<=i)return M(o.slice(),n);let a=Math.ceil(i/2),l=o.slice(0,a),p=o.slice(a);M(p,n);let h=p.slice(0,i-l.length);return M(l.concat(h),n)}var ne={winnerIndex:-1,steps:[],totalDurationMs:0};function U(s,t,e={}){var R,y,w,c;let i=Math.max(0,Math.floor(s));if(i===0)return ne;let r=(R=e.minLoops)!=null?R:1,o=(y=e.maxLoops)!=null?y:2,n=(w=e.growthFactor)!=null?w:1.08,a=(c=e.totalDurationMs)!=null?c:2600,l=P(t),p=Math.floor(l()*i),u=(r+Math.floor(l()*(o-r+1)))*i+p+1,d=[],m=0,v=1;for(let f=0;f<u;f++)d.push(m),m+=v,v*=n;let g=d[d.length-1]||1,x=[],b=-1;for(let f=0;f<u;f++){let L=Math.round(a*d[f]/g);L<=b&&(L=b+1),b=L,x.push({atMs:L,index:f%i})}return{winnerIndex:p,steps:x,totalDurationMs:b}}var E={colSpacing:1,rowRise:.42,rowScaleFalloff:.82,rowStaggerFraction:.5,posterAspect:.6666666666666666};var ae={cta:{en:"Tonight, what are we watching?",fr:"Ce soir, on regarde ?",es:"\xBFQu\xE9 vemos esta noche?",de:"Was schauen wir heute?"},spinning:{en:"Picking something\u2026",fr:"S\xE9lection en cours\u2026",es:"Eligiendo algo\u2026",de:"W\xE4hle etwas aus\u2026"},play_hint:{en:"Tap to watch",fr:"Toucher pour regarder",es:"Toca para ver",de:"Zum Ansehen tippen"},retry:{en:"Spin again",fr:"Relancer",es:"Girar de nuevo",de:"Nochmal drehen"},watch_now:{en:"Watch now",fr:"Regarder maintenant",es:"Ver ahora",de:"Jetzt ansehen"},opening_in:{en:"Opening in {s}s",fr:"Ouverture dans {s} s",es:"Abriendo en {s} s",de:"\xD6ffnet in {s} s"},empty:{en:"Add movies or shows to your library to light up the wall.",fr:"Ajoute des films ou s\xE9ries \xE0 ta biblioth\xE8que pour allumer le mur.",es:"A\xF1ade pel\xEDculas o series a tu biblioteca para encender el mural.",de:"F\xFCge Filme oder Serien hinzu, um die Wand zu erleuchten."}};function S(s,t){var r;let e=ae[s];if(!e)return s;let i=(t!=null?t:"en").slice(0,2).toLowerCase();return(r=e[i])!=null?r:e.en}var B=`
.mq-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: #05070a;
}
.mq-scene { position: absolute; inset: 0; }
.mq-cta {
  position: absolute;
  left: 50%;
  bottom: clamp(10px, 5cqi, 22px);
  transform: translateX(-50%);
  /* Above the hero veil (z-index 3) so "reroll" stays clickable while the
     elected pick is held. */
  z-index: 4;
  padding: clamp(6px, 2cqi, 10px) clamp(14px, 4cqi, 22px);
  border-radius: 999px;
  border: 1px solid rgba(255,255,255,0.28);
  background: rgba(10,10,14,0.55);
  backdrop-filter: blur(6px);
  color: #f5efe0;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3.2cqi, 14px);
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: background 150ms ease, transform 150ms ease, opacity 150ms ease;
  user-select: none;
}
.mq-cta:hover { background: rgba(30,24,16,0.7); transform: translateX(-50%) scale(1.03); }
.mq-cta[data-busy="true"] { opacity: 0.65; pointer-events: none; }
.mq-title {
  position: absolute;
  left: 50%;
  top: clamp(8px, 4cqi, 16px);
  transform: translateX(-50%);
  max-width: 80%;
  color: #ffffff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3.4cqi, 15px);
  font-weight: 600;
  letter-spacing: 0.02em;
  text-align: center;
  text-shadow: 0 1px 4px rgba(0,0,0,0.85);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
  opacity: 0;
  transition: opacity 120ms ease;
}
.mq-title.visible { opacity: 1; }
.mq-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 10%;
  text-align: center;
  color: rgba(245,239,224,0.75);
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3.2cqi, 14px);
  pointer-events: none;
}
.mq-hero {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: clamp(14px, 4cqi, 40px);
  padding: clamp(10px, 3cqi, 26px) clamp(14px, 5cqi, 60px);
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition: opacity 200ms ease, visibility 0s linear 200ms;
}
.mq-hero.visible { opacity: 1; visibility: visible; transition: opacity 200ms ease; }
/* The dimming veil sits behind the card so the blown-up poster reads clearly. */
.mq-hero::before {
  content: "";
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at center, rgba(4,5,9,0.62), rgba(3,4,7,0.9));
  backdrop-filter: blur(4px);
  opacity: 0;
  transition: opacity 200ms ease;
}
.mq-hero.preview::before { opacity: 0.7; }
.mq-hero.pick::before { opacity: 1; }
/* preview follows the mouse: never eat pointer events, keep the wall hoverable.
   pick is a deliberate stop: the card + its button become interactive. */
.mq-hero.pick { pointer-events: auto; }
.mq-hero-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: clamp(14px, 4cqi, 40px);
  max-width: 92%;
}
.mq-hero-poster {
  flex: 0 0 auto;
  height: clamp(150px, 70vh, 300px);
  aspect-ratio: 2 / 3;
  object-fit: cover;
  border-radius: 12px;
  background: #0c0f14;
  box-shadow: 0 18px 48px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.12), 0 0 60px rgba(120,150,255,0.12);
  transform: scale(0.86) translateY(6px);
  transition: transform 260ms cubic-bezier(0.22, 1, 0.36, 1);
}
.mq-hero.visible .mq-hero-poster { transform: scale(1) translateY(0); }
.mq-hero-info {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: clamp(6px, 1.6cqi, 14px);
  min-width: 0;
  max-width: min(46%, 420px);
}
.mq-hero-title {
  color: #fff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(16px, 4.6cqi, 30px);
  font-weight: 700;
  line-height: 1.12;
  letter-spacing: 0.01em;
  text-shadow: 0 2px 10px rgba(0,0,0,0.85);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.mq-hero-count {
  display: none;
  align-items: center;
  gap: 8px;
  color: rgba(245,239,224,0.82);
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3cqi, 15px);
  font-variant-numeric: tabular-nums;
}
.mq-hero-count::before {
  content: "";
  width: 7px; height: 7px;
  border-radius: 50%;
  background: #8ab4ff;
  box-shadow: 0 0 8px #8ab4ff;
  animation: mq-pulse 1s ease-in-out infinite;
}
@keyframes mq-pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.35; } }
.mq-hero.pick .mq-hero-count { display: flex; }
.mq-hero-watch {
  display: none;
  margin-top: clamp(2px, 1cqi, 8px);
  padding: clamp(8px, 2.2cqi, 12px) clamp(16px, 4.5cqi, 26px);
  border-radius: 999px;
  border: 1px solid rgba(255,255,255,0.16);
  background: linear-gradient(135deg, #f5efe0, #d8cbb0);
  color: #141210;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(12px, 3.4cqi, 16px);
  font-weight: 700;
  letter-spacing: 0.01em;
  cursor: pointer;
  transition: transform 140ms ease, box-shadow 140ms ease;
  box-shadow: 0 6px 20px rgba(0,0,0,0.4);
}
.mq-hero-watch:hover { transform: translateY(-1px) scale(1.03); box-shadow: 0 10px 26px rgba(0,0,0,0.5); }
.mq-hero.pick .mq-hero-watch { display: inline-block; }
@media (max-width: 520px) {
  .mq-hero-card { flex-direction: column; gap: 10px; }
  .mq-hero-poster { height: clamp(120px, 46vh, 190px); }
  .mq-hero-info { align-items: center; text-align: center; max-width: 88%; }
}
`,le=B.replace(/cqi/g,"vw");function G(s){var y,w;let t=document.createElement("style"),e=((w=(y=window.CSS)==null?void 0:y.supports)==null?void 0:w.call(y,"width: 1cqi"))===!0;t.textContent=e?`.mq-root { container-type: inline-size; }
${B}`:le,document.head.appendChild(t);let i=document.createElement("div");i.className="mq-root";let r=document.createElement("div");r.className="mq-scene",i.appendChild(r);let o=document.createElement("div");o.className="mq-title",i.appendChild(o);let n=document.createElement("button");n.type="button",n.className="mq-cta",i.appendChild(n);let a=document.createElement("div");a.className="mq-empty",a.style.display="none",i.appendChild(a);let l=document.createElement("div");l.className="mq-hero";let p=document.createElement("div");p.className="mq-hero-card";let h=document.createElement("img");h.className="mq-hero-poster",h.alt="",h.decoding="async";let u=document.createElement("div");u.className="mq-hero-info";let d=document.createElement("div");d.className="mq-hero-title";let m=document.createElement("div");m.className="mq-hero-count";let v=document.createElement("button");v.type="button",v.className="mq-hero-watch",u.appendChild(d),u.appendChild(m),u.appendChild(v),p.appendChild(h),p.appendChild(u),l.appendChild(p),i.appendChild(l),s.appendChild(i);let g=null,x=()=>g==null?void 0:g();n.addEventListener("click",x);let b=null,R=()=>b==null?void 0:b();return v.addEventListener("click",R),{canvasHost:r,setCtaLabel(c){n.textContent=c},setCtaBusy(c){n.dataset.busy=c?"true":"false"},onCtaClick(c){g=c},setTitle(c){if(!c){o.classList.remove("visible");return}o.textContent=c,o.classList.add("visible")},setEmptyMessage(c){a.textContent=c!=null?c:"",a.style.display=c?"flex":"none",n.style.display=c?"none":"block"},showHero(c,f,L){d.textContent=L,f&&h.src!==f&&(h.src=f),h.style.visibility=f?"visible":"hidden",c==="pick"?(l.classList.add("pick"),l.classList.remove("preview")):(l.classList.add("preview"),l.classList.remove("pick"),m.textContent=""),l.classList.add("visible")},setHeroCountdown(c){m.textContent=c!=null?c:""},setHeroWatchLabel(c){v.textContent=c},onHeroWatch(c){b=c},hideHero(){l.classList.remove("visible","pick","preview"),m.textContent=""},dispose(){n.removeEventListener("click",x),v.removeEventListener("click",R),i.remove(),t.remove()}}}var ce=30*60*1e3,ue=8e3,A=class{constructor(t){this.deps=t;this.overlay=null;this.init=null;this.posters=[];this.posterUrls=[];this.spinning=!1;this.picking=!1;this.pickedId="";this.countdownTimer=0;this.landDeadline=0;this.refreshTimer=0;this.disposed=!1;this.booted=!1;this.onPageHide=()=>this.dispose();this.onResize=()=>this.requestHeight();this.onVisible=()=>{document.hidden||this.loadLibrary()}}start(){this.deps.bridge.onReady(t=>void this.boot(t))}dispose(){var t;this.disposed||(this.disposed=!0,clearInterval(this.refreshTimer),clearInterval(this.countdownTimer),window.removeEventListener("pagehide",this.onPageHide),window.removeEventListener("resize",this.onResize),document.removeEventListener("visibilitychange",this.onVisible),this.deps.renderer.dispose(),(t=this.overlay)==null||t.dispose(),this.overlay=null)}async boot(t){this.booted||(this.booted=!0,this.init=t,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent",document.body.style.overflow="hidden",this.overlay=G(document.body),this.deps.renderer.mount(this.overlay.canvasHost),this.overlay.setCtaLabel(S("cta",t.locale)),this.overlay.onCtaClick(()=>this.spin()),this.overlay.setHeroWatchLabel(S("watch_now",t.locale)),this.overlay.onHeroWatch(()=>this.watchPicked()),this.deps.renderer.onActivate(e=>this.activate(e)),this.deps.renderer.onHover(e=>this.hover(e)),this.requestHeight(),window.addEventListener("resize",this.onResize),window.addEventListener("pagehide",this.onPageHide),document.addEventListener("visibilitychange",this.onVisible),await this.loadLibrary(),!this.disposed&&(this.refreshTimer=window.setInterval(()=>void this.loadLibrary(),ce)))}requestHeight(){let t=window.innerWidth||document.documentElement.clientWidth||800,e=t<420?220:t<700?280:340;this.deps.bridge.resize(e)}async loadLibrary(){var n;if(this.disposed||!this.overlay)return;let t=await this.deps.bridge.listMedia();if(this.disposed)return;let e=this.deps.renderer.getCapacity(),i=(Date.now()^t.length*2654435761)>>>0;if(this.posters=D(t,e.posterCount,i),this.posters.length===0){this.overlay.setEmptyMessage(S("empty",(n=this.init)==null?void 0:n.locale));return}this.overlay.setEmptyMessage(null);let r=O({count:this.posters.length,rows:e.rows,colSpacing:E.colSpacing,rowRise:E.rowRise,rowScaleFalloff:E.rowScaleFalloff,rowStaggerFraction:E.rowStaggerFraction}),o=await Promise.all(this.posters.map(a=>this.deps.bridge.readPoster(a.id).catch(()=>"")));this.disposed||(this.posterUrls=o,this.deps.renderer.setWall({layout:r,posters:this.posters.map((a,l)=>({id:a.id,title:a.title,posterUrl:o[l]||""}))}))}spin(){var e;if(this.spinning||this.posters.length===0||!this.overlay)return;this.stopCountdown(),this.picking=!1,this.overlay.hideHero();let t=U(this.posters.length,Date.now());t.winnerIndex<0||(this.spinning=!0,this.overlay.setCtaBusy(!0),this.overlay.setCtaLabel(S("spinning",(e=this.init)==null?void 0:e.locale)),this.deps.renderer.runRoulette(t,i=>this.landed(i)))}landed(t){var i,r;if(this.spinning=!1,this.disposed||!this.overlay)return;this.overlay.setCtaBusy(!1);let e=this.posters[t];if(!e){this.overlay.setCtaLabel(S("cta",(i=this.init)==null?void 0:i.locale));return}this.picking=!0,this.pickedId=e.id,this.overlay.showHero("pick",this.posterUrls[t]||"",e.title),this.overlay.setCtaLabel(S("retry",(r=this.init)==null?void 0:r.locale)),this.landDeadline=Date.now()+ue,this.tickCountdown(e.id),this.stopCountdown(),this.countdownTimer=window.setInterval(()=>this.tickCountdown(e.id),200)}tickCountdown(t){var r;if(this.disposed||!this.overlay)return;let e=this.landDeadline-Date.now();if(e<=0){this.stopCountdown(),this.goToMedia(t);return}let i=Math.ceil(e/1e3);this.overlay.setHeroCountdown(S("opening_in",(r=this.init)==null?void 0:r.locale).replace("{s}",String(i)))}stopCountdown(){clearInterval(this.countdownTimer),this.countdownTimer=0}watchPicked(){this.picking&&this.pickedId&&this.goToMedia(this.pickedId)}hover(t){if(!this.overlay||this.picking)return;if(t===null){this.overlay.hideHero();return}let e=this.posters[t];if(!e){this.overlay.hideHero();return}this.overlay.showHero("preview",this.posterUrls[t]||"",e.title)}activate(t){let e=this.posters[t];e&&this.goToMedia(e.id)}goToMedia(t){this.stopCountdown(),this.picking=!1,this.disposed||this.deps.bridge.navigate(`/media/${t}`)}};function pe(s){return!s||typeof s.id!="string"||!s.id||typeof s.posterUrl!="string"||!s.posterUrl?null:{id:s.id,title:typeof s.title=="string"?s.title:"",year:typeof s.year=="number"&&s.year>0?s.year:void 0,posterUrl:s.posterUrl}}var _=class{onReady(t){let e=(i,r)=>({locale:i!=null?i:"en",theme:r!=null?r:"dark"});typeof window!="undefined"&&window.evey?window.evey.onInit(i=>t(e(i==null?void 0:i.locale,i==null?void 0:i.theme))):t(e(navigator==null?void 0:navigator.language,void 0))}resize(t){var e;(e=window.evey)==null||e.resize(Math.round(t))}navigate(t){var e;(e=window.evey)==null||e.navigate(t)}async readPoster(t){var i;let e=(i=window.evey)==null?void 0:i.media;if(!(e!=null&&e.poster)||!t)return"";try{let r=await e.poster(t);return typeof r=="string"?r:""}catch{return""}}async listMedia(){var e;let t=(e=window.evey)==null?void 0:e.media;if(!(t!=null&&t.list))return[];try{let i=await t.list({}),r=Array.isArray(i)?i:Array.isArray(i==null?void 0:i.items)?i.items:[],o=[];for(let n of r){let a=pe(n);a&&o.push(a)}return o}catch{return[]}}};function z(s,t,e,i){if(!(i>0))return 0;let r=(s+t*e)%i;return r<0?r+i:r}function N(s,t){if(!(t>0))return 0;let e=(s%t+t)%t/t;return e<.5?e*2:2-e*2}function X(s,t=1){return(s*2-1)*t}function V(s,t,e,i,r){let o=e>0?[-e,0,e]:[0],n=null;for(let a of s){let l=r.halfWidth*a.scale,p=r.halfHeight*a.scale;for(let h of o){let u=a.x-t+h;if(Math.abs(i.x-u)<=l&&Math.abs(i.y-a.y)<=p){n=a;break}}}return n?n.index:null}var Y=[[.55,.18,.16],[.16,.22,.42],[.3,.22,.1],[.14,.32,.28],[.35,.14,.3],[.22,.22,.22],[.45,.3,.1],[.12,.14,.24]];function K(s){let t=F(s!=null?s:"");return Y[t%Y.length]}var j=`
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`,Q=`
precision mediump float;
varying vec2 vUv;
uniform vec2 uRes;
uniform float uTime;
uniform float uSpotX;
uniform float uSpotIntensity;
uniform float uSpotWidth;
uniform float uLock;

void main() {
  vec2 uv = vUv;
  vec3 base = mix(vec3(0.015, 0.012, 0.018), vec3(0.09, 0.045, 0.03), uv.y);

  vec2 c = (uv - 0.5) * vec2(uRes.x / max(uRes.y, 1.0), 1.0);
  float vig = smoothstep(0.95, 0.2, length(c));
  vec3 col = base * mix(0.35, 1.0, vig);

  float dx = abs(uv.x - (uSpotX * 0.5 + 0.5));
  float beam = smoothstep(uSpotWidth, 0.0, dx) * (1.0 - uv.y * 0.35);
  float intensity = uSpotIntensity * mix(1.0, 1.9, uLock);
  col += vec3(1.0, 0.86, 0.56) * beam * intensity * 0.55;

  float n = fract(sin(dot(uv * uRes.xy + uTime * 53.0, vec2(12.9898, 78.233))) * 43758.5453);
  col += (n - 0.5) * 0.018;

  gl_FragColor = vec4(col, 1.0);
}
`,Z=`
attribute vec2 aCorner;
uniform vec2 uCenter;
uniform vec2 uSize;
uniform float uAspect;
varying vec2 vUv;

void main() {
  vUv = aCorner + 0.5;
  vec2 pos = uCenter + aCorner * uSize;
  gl_Position = vec4(pos.x / max(uAspect, 0.0001), pos.y, 0.0, 1.0);
}
`,$=`
precision mediump float;
varying vec2 vUv;
uniform sampler2D uTex;
uniform vec3 uPlaceholder;
uniform float uLoaded;
uniform float uDepthFade;
uniform float uHighlight;
uniform float uSpotLight;
uniform float uAlpha;

void main() {
  vec3 texColor = texture2D(uTex, vec2(vUv.x, 1.0 - vUv.y)).rgb;
  vec3 placeholderColor = uPlaceholder * (0.82 + 0.32 * vUv.y);
  vec3 col = mix(placeholderColor, texColor, uLoaded);
  col *= mix(0.5, 1.0, 1.0 - uDepthFade);
  col += uHighlight * 0.22;
  col *= (0.68 + 0.32 * uSpotLight);

  vec2 edge = abs(vUv - 0.5) * 2.0;
  float border = smoothstep(0.95, 1.0, max(edge.x, edge.y));
  col = mix(col, vec3(0.0), border * 0.55);

  gl_FragColor = vec4(col, uAlpha);
}
`;function J(s){let t=Math.max(1,Math.floor(s)),e=[],i=r=>e.indexOf(r);return{touch(r){let o=i(r);return o!==-1?(e.splice(o,1),e.push(r),null):(e.push(r),e.length>t?e.shift():null)},has(r){return i(r)!==-1},size(){return e.length},keys(){return e.slice()},remove(r){let o=i(r);o!==-1&&e.splice(o,1)}}}var he=4,k=class{constructor(t,e){this.gl=t;this.textures=new Map;this.queue=[];this.activeLoads=0;this.disposed=!1;this.lru=J(e)}reserve(t,e,i){let r=this.lru.touch(t);r!==null&&r!==t&&this.evict(r);let o=this.textures.get(t);return o||(o={tex:this.createPlaceholderTexture(i),loaded:!1},this.textures.set(t,o),e&&this.queue.push({id:t,url:e})),o.tex}isLoaded(t){var e,i;return(i=(e=this.textures.get(t))==null?void 0:e.loaded)!=null?i:!1}pump(){for(;this.activeLoads<he&&this.queue.length>0;){let t=this.queue.shift();if(!t)break;if(!this.textures.get(t.id))continue;this.activeLoads++;let i=new Image;i.onload=()=>{if(this.activeLoads--,this.disposed)return;let r=this.textures.get(t.id);if(!r)return;let o=this.gl;try{o.bindTexture(o.TEXTURE_2D,r.tex),o.pixelStorei(o.UNPACK_FLIP_Y_WEBGL,0),o.texImage2D(o.TEXTURE_2D,0,o.RGB,o.RGB,o.UNSIGNED_BYTE,i),o.pixelStorei(o.UNPACK_FLIP_Y_WEBGL,0),r.loaded=!0}catch(n){console.warn("marquee: poster texImage2D threw (tainted?)",n)}},i.onerror=()=>{this.activeLoads--},i.src=t.url}}dispose(){this.disposed=!0;for(let t of this.textures.values())this.gl.deleteTexture(t.tex);this.textures.clear(),this.queue.length=0}evict(t){let e=this.textures.get(t);e&&(this.gl.deleteTexture(e.tex),this.textures.delete(t))}createPlaceholderTexture(t){let e=this.gl,i=e.createTexture();if(!i)throw new Error("marquee: createTexture failed");e.bindTexture(e.TEXTURE_2D,i);let[r,o,n]=t;return e.texImage2D(e.TEXTURE_2D,0,e.RGB,1,1,0,e.RGB,e.UNSIGNED_BYTE,new Uint8Array([Math.round(r*255),Math.round(o*255),Math.round(n*255)])),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),i}};var T=E.colSpacing*.47,I=T/E.posterAspect,q=-.25,de=.09,me=1.16,fe=.15,ve=1.3,ge=.032,be=14e3,ee=.22,C=null;function xe(){var s;if(C)return C;try{let t=document.createElement("canvas"),e=t.getContext("webgl2")||t.getContext("webgl");if(!e)return C={scale:.5,fps:24,maxDpr:1,posterCount:14,rows:2,reflections:!1,textureCapacity:20};let i=e.getParameter(e.MAX_TEXTURE_SIZE),r=(s=navigator.hardwareConcurrency)!=null?s:2;return"drawBuffers"in e&&i>=8192&&r>=4?C={scale:1,fps:30,maxDpr:1.5,posterCount:30,rows:3,reflections:!0,textureCapacity:42}:i>=4096?C={scale:.75,fps:30,maxDpr:1.25,posterCount:24,rows:3,reflections:!0,textureCapacity:36}:C={scale:.5,fps:24,maxDpr:1,posterCount:14,rows:2,reflections:!1,textureCapacity:20}}catch{return C={scale:.5,fps:24,maxDpr:1,posterCount:14,rows:2,reflections:!1,textureCapacity:20}}}function te(s,t){if(!(t>0))return s;let e=(s%t+t)%t;return e>t/2&&(e-=t),e}function H(s,t,e){return s<t?t:s>e?e:s}var W=class{constructor(){this.host=null;this.canvas=null;this.gl=null;this.bgProgram=null;this.bgVbo=null;this.bgLoc={};this.posterProgram=null;this.quadVbo=null;this.posterLoc={};this.posterAttr=-1;this.textures=null;this.quality=xe();this.reducedMotion=typeof window!="undefined"&&window.matchMedia("(prefers-reduced-motion: reduce)").matches;this.slots=[];this.posters=[];this.period=1;this.scrollOffset=0;this.hoverIndex=null;this.hoverLift=0;this.lockLift=0;this.spin=null;this.spinStartMs=0;this.lockedIndex=null;this.onLandedCb=null;this.raf=0;this.running=!1;this.inView=!0;this.lastTs=0;this.frameAcc=0;this.elapsedMs=0;this.lost=!1;this.activateCb=null;this.hoverCb=null;this.tileOffsets=[0,0,0];this.spotX=0;this.spotIntensity=0;this.spotLock=0;this.resizeObs=null;this.viewObs=null;this.onLost=t=>{t.preventDefault(),this.lost=!0};this.onRestored=()=>{this.lost=!1,this.initGL()};this.onPointerMove=t=>this.handlePointer(t,!1);this.onPointerUp=t=>this.handlePointer(t,!0);this.onPointerLeave=()=>this.setHover(null);this.loop=t=>{var a;if(!this.running)return;if(this.raf=requestAnimationFrame(this.loop),document.hidden||!this.inView||this.lost||!this.gl||!this.bgProgram||!this.posterProgram){this.lastTs=t;return}let e=Math.min((t-this.lastTs)/1e3,.25);this.lastTs=t,this.frameAcc+=e;let i=1/this.quality.fps;if(this.frameAcc<i)return;let r=Math.min(this.frameAcc,i*3);this.frameAcc=0;let o=this.reducedMotion?.25:1;this.elapsedMs+=r*o*1e3,this.spin!==null||this.lockedIndex!==null||(this.scrollOffset=z(this.scrollOffset,r*o,ge,this.period)),this.hoverLift+=((this.hoverIndex!==null?1:0)-this.hoverLift)*(1-Math.exp(-8*r)),this.lockLift+=((this.lockedIndex!==null?1:0)-this.lockLift)*(1-Math.exp(-6*r)),(a=this.textures)==null||a.pump(),this.draw()}}getCapacity(){let t=this.quality.rows,e=this.aspect(),i=Math.ceil(2*e)+2,r=Math.ceil(this.quality.posterCount/t),o=Math.max(r,i);return{posterCount:Math.min(o*t,this.quality.textureCapacity),rows:t}}mount(t){this.host=t;let e=document.createElement("canvas");e.style.cssText="position:absolute;inset:0;width:100%;height:100%;display:block;cursor:pointer;",t.appendChild(e),this.canvas=e,e.addEventListener("webglcontextlost",this.onLost,!1),e.addEventListener("webglcontextrestored",this.onRestored,!1),e.addEventListener("pointermove",this.onPointerMove),e.addEventListener("pointerup",this.onPointerUp),e.addEventListener("pointerleave",this.onPointerLeave),this.initGL(),this.resizeObs=new ResizeObserver(()=>this.resize()),this.resizeObs.observe(t),this.viewObs=new IntersectionObserver(i=>{var r,o;this.inView=(o=(r=i[0])==null?void 0:r.isIntersecting)!=null?o:!0}),this.viewObs.observe(t),this.resize(),this.running=!0,this.raf=requestAnimationFrame(this.loop)}setWall(t){this.slots=t.layout.slots,this.period=t.layout.period,this.posters=t.posters.map(e=>{let i=K(e.id),r=this.textures?this.textures.reserve(e.id,e.posterUrl,i):null;return{id:e.id,posterUrl:e.posterUrl,placeholder:i,tex:r}}),this.scrollOffset=0,this.hoverIndex=null,this.lockedIndex=null,this.spin=null}runRoulette(t,e){this.slots.length===0||t.steps.length===0||this.spin||(this.spin=t,this.spinStartMs=this.elapsedMs,this.lockedIndex=null,this.onLandedCb=e)}onActivate(t){this.activateCb=t}onHover(t){this.hoverCb=t}dispose(){var i,r,o,n;this.running=!1,cancelAnimationFrame(this.raf),(i=this.resizeObs)==null||i.disconnect(),(r=this.viewObs)==null||r.disconnect(),this.resizeObs=null,this.viewObs=null;let{gl:t,canvas:e}=this;e&&(e.removeEventListener("webglcontextlost",this.onLost),e.removeEventListener("webglcontextrestored",this.onRestored),e.removeEventListener("pointermove",this.onPointerMove),e.removeEventListener("pointerup",this.onPointerUp),e.removeEventListener("pointerleave",this.onPointerLeave)),(o=this.textures)==null||o.dispose(),this.textures=null,t&&(this.bgProgram&&t.deleteProgram(this.bgProgram),this.posterProgram&&t.deleteProgram(this.posterProgram),this.bgVbo&&t.deleteBuffer(this.bgVbo),this.quadVbo&&t.deleteBuffer(this.quadVbo),(n=t.getExtension("WEBGL_lose_context"))==null||n.loseContext()),e==null||e.remove(),this.gl=null,this.bgProgram=null,this.posterProgram=null,this.bgVbo=null,this.quadVbo=null,this.canvas=null,this.host=null}compile(t,e){let i=this.gl;if(!i)return null;let r=i.createShader(t);return r?(i.shaderSource(r,e),i.compileShader(r),i.getShaderParameter(r,i.COMPILE_STATUS)?r:(console.error("marquee shader:",i.getShaderInfoLog(r)),i.deleteShader(r),null)):null}link(t,e){let i=this.gl;if(!i)return null;let r=this.compile(i.VERTEX_SHADER,t),o=this.compile(i.FRAGMENT_SHADER,e);if(!r||!o)return null;let n=i.createProgram();return n?(i.attachShader(n,r),i.attachShader(n,o),i.linkProgram(n),i.deleteShader(r),i.deleteShader(o),i.getProgramParameter(n,i.LINK_STATUS)?n:(console.error("marquee link:",i.getProgramInfoLog(n)),i.deleteProgram(n),null)):null}initGL(){let t=this.canvas;if(!t)return;let e=t.getContext("webgl",{alpha:!0,premultipliedAlpha:!1,antialias:!1,depth:!1,stencil:!1,preserveDrawingBuffer:!1,powerPreference:"low-power"});if(e&&(this.gl=e,this.bgProgram=this.link(j,Q),this.posterProgram=this.link(Z,$),!(!this.bgProgram||!this.posterProgram))){this.bgVbo=e.createBuffer(),e.bindBuffer(e.ARRAY_BUFFER,this.bgVbo),e.bufferData(e.ARRAY_BUFFER,new Float32Array([-1,-1,3,-1,-1,3]),e.STATIC_DRAW);for(let i of["uRes","uTime","uSpotX","uSpotIntensity","uSpotWidth","uLock"])this.bgLoc[i]=e.getUniformLocation(this.bgProgram,i);this.quadVbo=e.createBuffer(),e.bindBuffer(e.ARRAY_BUFFER,this.quadVbo),e.bufferData(e.ARRAY_BUFFER,new Float32Array([-.5,-.5,.5,-.5,-.5,.5,.5,.5]),e.STATIC_DRAW),this.posterAttr=e.getAttribLocation(this.posterProgram,"aCorner");for(let i of["uCenter","uSize","uAspect","uTex","uPlaceholder","uLoaded","uDepthFade","uHighlight","uSpotLight","uAlpha"])this.posterLoc[i]=e.getUniformLocation(this.posterProgram,i);this.textures=new k(e,this.quality.textureCapacity),e.disable(e.DEPTH_TEST),e.clearColor(.02,.015,.02,1)}}resize(){let{host:t,canvas:e,gl:i}=this;if(!t||!e||!i)return;let r=Math.min(window.devicePixelRatio||1,this.quality.maxDpr)*this.quality.scale,o=Math.max(1,Math.round(t.clientWidth*r)),n=Math.max(1,Math.round(t.clientHeight*r));(e.width!==o||e.height!==n)&&(e.width=o,e.height=n,i.viewport(0,0,o,n))}aspect(){let t=this.canvas;return t&&t.height>0?t.width/t.height:1}setHover(t){var e;this.hoverIndex!==t&&(this.hoverIndex=t,(e=this.hoverCb)==null||e.call(this,t))}handlePointer(t,e){var u,d;let i=this.canvas;if(!i||this.slots.length===0)return;let r=i.getBoundingClientRect();if(r.width<=0||r.height<=0)return;let o=(t.clientX-r.left)/r.width*2-1,n=-((t.clientY-r.top)/r.height*2-1),a=this.aspect(),l=o*a,p=n-q,h=V(this.slots,this.scrollOffset,this.period,{x:l,y:p},{halfWidth:T,halfHeight:I});if(e){let m=(u=this.hoverIndex)!=null?u:h;m!==null&&!this.spin&&((d=this.activateCb)==null||d.call(this,m))}else this.setHover(h)}updateSpotlight(){let t=this.aspect();if(this.spin){let i=this.spin,r=this.elapsedMs-this.spinStartMs;if(r>=i.totalDurationMs){this.lockedIndex=i.winnerIndex,this.spin=null;let o=this.onLandedCb;this.onLandedCb=null,o==null||o(this.lockedIndex)}else{let o=0;for(let a=0;a<i.steps.length&&i.steps[a].atMs<=r;a++)o=a;let n=this.slots[i.steps[o].index];this.spotX=n?H(te(n.x-this.scrollOffset,this.period)/t,-1,1):0,this.spotIntensity=1,this.spotLock=.4;return}}if(this.lockedIndex!==null){let i=this.slots[this.lockedIndex];this.spotX=i?H(te(i.x-this.scrollOffset,this.period)/t,-1,1):0,this.spotIntensity=1,this.spotLock=1;return}let e=N(this.elapsedMs,be);this.spotX=X(e,1),this.spotIntensity=.55,this.spotLock=0}draw(){var n,a;let t=this.gl,e=this.canvas;if(!t||!e||!this.bgProgram||!this.posterProgram)return;this.updateSpotlight();let i=this.aspect();t.clear(t.COLOR_BUFFER_BIT),t.useProgram(this.bgProgram),t.bindBuffer(t.ARRAY_BUFFER,this.bgVbo);let r=t.getAttribLocation(this.bgProgram,"aPos");if(t.enableVertexAttribArray(r),t.vertexAttribPointer(r,2,t.FLOAT,!1,0,0),t.uniform2f(this.bgLoc.uRes,e.width,e.height),t.uniform1f(this.bgLoc.uTime,this.elapsedMs/1e3),t.uniform1f(this.bgLoc.uSpotX,this.spotX),t.uniform1f(this.bgLoc.uSpotIntensity,this.spotIntensity),t.uniform1f(this.bgLoc.uSpotWidth,ee),t.uniform1f(this.bgLoc.uLock,this.spotLock),t.drawArrays(t.TRIANGLES,0,3),t.disableVertexAttribArray(r),this.posters.length===0)return;t.useProgram(this.posterProgram),t.bindBuffer(t.ARRAY_BUFFER,this.quadVbo),t.enableVertexAttribArray(this.posterAttr),t.vertexAttribPointer(this.posterAttr,2,t.FLOAT,!1,0,0),t.uniform1f(this.posterLoc.uAspect,i),t.uniform1i(this.posterLoc.uTex,0),t.activeTexture(t.TEXTURE0),t.uniform1f(this.posterLoc.uAlpha,1);let o=i+T;this.tileOffsets[0]=-this.period,this.tileOffsets[1]=0,this.tileOffsets[2]=this.period;for(let l=0;l<this.posters.length;l++){let p=this.slots[l],h=this.posters[l];if(!p||!h)continue;let u=h.tex,d=(a=(n=this.textures)==null?void 0:n.isLoaded(h.id))!=null?a:!1,m=l===this.hoverIndex,v=l===this.lockedIndex,g=m?this.hoverLift:0,x=v?this.lockLift:0,b=Math.max(g,x),R=x>=g?ve:me,y=x>=g?fe:de,w=1+(R-1)*b;for(let c of this.tileOffsets){let f=p.x-this.scrollOffset+c;if(Math.abs(f)>o)continue;let L=p.y+q+y*b,ie=T*p.scale*w,re=I*p.scale*w,oe=H(f/i,-1,1),se=H(1-Math.abs(oe-this.spotX)/ee,0,1);u&&t.bindTexture(t.TEXTURE_2D,u),t.uniform2f(this.posterLoc.uCenter,f,L),t.uniform2f(this.posterLoc.uSize,ie,re),t.uniform3f(this.posterLoc.uPlaceholder,h.placeholder[0],h.placeholder[1],h.placeholder[2]),t.uniform1f(this.posterLoc.uLoaded,d?1:0),t.uniform1f(this.posterLoc.uDepthFade,p.z),t.uniform1f(this.posterLoc.uHighlight,b),t.uniform1f(this.posterLoc.uSpotLight,se),t.drawArrays(t.TRIANGLE_STRIP,0,4)}}this.quality.reflections&&this.drawReflections(o),t.disableVertexAttribArray(this.posterAttr)}drawReflections(t){var r,o;let e=this.gl;if(!e)return;let i=q-I;e.enable(e.BLEND),e.blendFunc(e.SRC_ALPHA,e.ONE_MINUS_SRC_ALPHA),e.uniform1f(this.posterLoc.uAlpha,.14),e.uniform1f(this.posterLoc.uHighlight,0),e.uniform1f(this.posterLoc.uSpotLight,.6);for(let n=0;n<this.posters.length;n++){let a=this.slots[n],l=this.posters[n];if(!a||a.row!==0||!l)continue;let p=l.tex,h=(o=(r=this.textures)==null?void 0:r.isLoaded(l.id))!=null?o:!1,u=T*a.scale,d=I*a.scale,m=2*i-(a.y+q);for(let v of this.tileOffsets){let g=a.x-this.scrollOffset+v;Math.abs(g)>t||(p&&e.bindTexture(e.TEXTURE_2D,p),e.uniform2f(this.posterLoc.uCenter,g,m),e.uniform2f(this.posterLoc.uSize,u,d),e.uniform3f(this.posterLoc.uPlaceholder,l.placeholder[0],l.placeholder[1],l.placeholder[2]),e.uniform1f(this.posterLoc.uLoaded,h?1:0),e.uniform1f(this.posterLoc.uDepthFade,a.z),e.drawArrays(e.TRIANGLE_STRIP,0,4))}}e.disable(e.BLEND)}};new A({renderer:new W,bridge:new _}).start();})();
