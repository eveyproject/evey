"use strict";(()=>{function U(s){let t=Math.max(0,Math.floor(s.count)),e=Math.max(1,Math.floor(s.rows)),r=t===0?1:Math.max(1,Math.ceil(t/e)),i=r*s.colSpacing,o=[];for(let n=0;n<t;n++){let a=Math.floor(n/r),l=n%r,c=a%2===1?s.colSpacing*s.rowStaggerFraction:0,u=(l-(r-1)/2)*s.colSpacing+c,p=a*s.rowRise,h=e>1?a/(e-1):0,d=Math.pow(s.rowScaleFalloff,a);o.push({index:n,row:a,col:l,x:u,y:p,scale:d,z:h})}return{slots:o,rows:e,colsPerRow:r,period:i>0?i:s.colSpacing}}function T(s){let t=s>>>0;return function(){t|=0,t=t+1831565813|0;let r=Math.imul(t^t>>>15,1|t);return r=r+Math.imul(r^r>>>7,61|r)^r,((r^r>>>14)>>>0)/4294967296}}function F(s){let t=2166136261;for(let e=0;e<s.length;e++)t^=s.charCodeAt(e),t=Math.imul(t,16777619);return t>>>0}function P(s,t){for(let e=s.length-1;e>0;e--){let r=Math.floor(t()*(e+1)),i=s[e];s[e]=s[r],s[r]=i}return s}function B(s,t,e){let r=Math.max(0,Math.floor(t));if(r===0)return[];let i=new Set,o=[];for(let p of s)!p||!p.id||!p.posterUrl||i.has(p.id)||(i.add(p.id),o.push(p));let n=T(e);if(o.length<=r)return P(o.slice(),n);let a=Math.ceil(r/2),l=o.slice(0,a),c=o.slice(a);P(c,n);let u=c.slice(0,r-l.length);return P(l.concat(u),n)}var re={winnerIndex:-1,steps:[],totalDurationMs:0};function D(s,t,e={}){var b,w,S,R,E;let r=Math.max(0,Math.floor(s));if(r===0)return re;let i=(b=e.minLoops)!=null?b:2,o=(w=e.maxLoops)!=null?w:4,n=(S=e.growthFactor)!=null?S:1.15,a=(R=e.minIntervalMs)!=null?R:70,l=Math.max(a,(E=e.startIntervalMs)!=null?E:70),c=T(t),u=Math.floor(c()*r),h=(i+Math.floor(c()*(o-i+1)))*r+u+1,d=[],m=0,f=l;for(let y=0;y<h;y++){d.push({atMs:Math.round(m),index:y%r});let H=y/h,te=H>.55?n+(H-.55)*1.8:n;f=Math.max(a,f*te),m+=f}return{winnerIndex:u,steps:d,totalDurationMs:Math.round(m)}}var v={colSpacing:1,rowRise:.42,rowScaleFalloff:.82,rowStaggerFraction:.5,posterAspect:.6666666666666666};var ie={cta:{en:"Tonight, what are we watching?",fr:"Ce soir, on regarde ?",es:"\xBFQu\xE9 vemos esta noche?",de:"Was schauen wir heute?"},spinning:{en:"Picking something\u2026",fr:"S\xE9lection en cours\u2026",es:"Eligiendo algo\u2026",de:"W\xE4hle etwas aus\u2026"},play_hint:{en:"Tap to watch",fr:"Toucher pour regarder",es:"Toca para ver",de:"Zum Ansehen tippen"},empty:{en:"Add movies or shows to your library to light up the wall.",fr:"Ajoute des films ou s\xE9ries \xE0 ta biblioth\xE8que pour allumer le mur.",es:"A\xF1ade pel\xEDculas o series a tu biblioteca para encender el mural.",de:"F\xFCge Filme oder Serien hinzu, um die Wand zu erleuchten."}};function x(s,t){var i;let e=ie[s];if(!e)return s;let r=(t!=null?t:"en").slice(0,2).toLowerCase();return(i=e[r])!=null?i:e.en}var q=`
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
`,oe=q.replace(/cqi/g,"vw");function G(s){var u,p;let t=document.createElement("style"),e=((p=(u=window.CSS)==null?void 0:u.supports)==null?void 0:p.call(u,"width: 1cqi"))===!0;t.textContent=e?`.mq-root { container-type: inline-size; }
${q}`:oe,document.head.appendChild(t);let r=document.createElement("div");r.className="mq-root";let i=document.createElement("div");i.className="mq-scene",r.appendChild(i);let o=document.createElement("div");o.className="mq-title",r.appendChild(o);let n=document.createElement("button");n.type="button",n.className="mq-cta",r.appendChild(n);let a=document.createElement("div");a.className="mq-empty",a.style.display="none",r.appendChild(a),s.appendChild(r);let l=null,c=()=>l==null?void 0:l();return n.addEventListener("click",c),{canvasHost:i,setCtaLabel(h){n.textContent=h},setCtaBusy(h){n.dataset.busy=h?"true":"false"},onCtaClick(h){l=h},setTitle(h){if(!h){o.classList.remove("visible");return}o.textContent=h,o.classList.add("visible")},setEmptyMessage(h){a.textContent=h!=null?h:"",a.style.display=h?"flex":"none",n.style.display=h?"none":"block"},dispose(){n.removeEventListener("click",c),r.remove(),t.remove()}}}var se=30*60*1e3,C=class{constructor(t){this.deps=t;this.overlay=null;this.init=null;this.posters=[];this.spinning=!1;this.refreshTimer=0;this.disposed=!1;this.booted=!1;this.onPageHide=()=>this.dispose();this.onResize=()=>this.requestHeight();this.onVisible=()=>{document.hidden||this.loadLibrary()}}start(){this.deps.bridge.onReady(t=>void this.boot(t))}dispose(){var t;this.disposed||(this.disposed=!0,clearInterval(this.refreshTimer),window.removeEventListener("pagehide",this.onPageHide),window.removeEventListener("resize",this.onResize),document.removeEventListener("visibilitychange",this.onVisible),this.deps.renderer.dispose(),(t=this.overlay)==null||t.dispose(),this.overlay=null)}async boot(t){this.booted||(this.booted=!0,this.init=t,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent",document.body.style.overflow="hidden",this.overlay=G(document.body),this.deps.renderer.mount(this.overlay.canvasHost),this.overlay.setCtaLabel(x("cta",t.locale)),this.overlay.onCtaClick(()=>this.spin()),this.deps.renderer.onActivate(e=>this.activate(e)),this.deps.renderer.onHover(e=>this.hover(e)),this.requestHeight(),window.addEventListener("resize",this.onResize),window.addEventListener("pagehide",this.onPageHide),document.addEventListener("visibilitychange",this.onVisible),await this.loadLibrary(),!this.disposed&&(this.refreshTimer=window.setInterval(()=>void this.loadLibrary(),se)))}requestHeight(){let t=window.innerWidth||document.documentElement.clientWidth||800,e=t<420?220:t<700?280:340;this.deps.bridge.resize(e)}async loadLibrary(){var o;if(this.disposed||!this.overlay)return;let t=await this.deps.bridge.listMedia();if(this.disposed)return;let e=this.deps.renderer.getCapacity(),r=(Date.now()^t.length*2654435761)>>>0;if(this.posters=B(t,e.posterCount,r),this.posters.length===0){this.overlay.setEmptyMessage(x("empty",(o=this.init)==null?void 0:o.locale));return}this.overlay.setEmptyMessage(null);let i=U({count:this.posters.length,rows:e.rows,colSpacing:v.colSpacing,rowRise:v.rowRise,rowScaleFalloff:v.rowScaleFalloff,rowStaggerFraction:v.rowStaggerFraction});this.deps.renderer.setWall({layout:i,posters:this.posters.map(n=>({id:n.id,title:n.title,posterUrl:n.posterUrl}))})}spin(){var e;if(this.spinning||this.posters.length===0||!this.overlay)return;let t=D(this.posters.length,Date.now());t.winnerIndex<0||(this.spinning=!0,this.overlay.setCtaBusy(!0),this.overlay.setCtaLabel(x("spinning",(e=this.init)==null?void 0:e.locale)),this.deps.renderer.runRoulette(t,r=>this.landed(r)))}landed(t){var r,i;if(this.spinning=!1,this.disposed||!this.overlay)return;this.overlay.setCtaBusy(!1),this.overlay.setCtaLabel(x("cta",(r=this.init)==null?void 0:r.locale));let e=this.posters[t];e&&this.overlay.setTitle(`${e.title} \xB7 ${x("play_hint",(i=this.init)==null?void 0:i.locale)}`)}hover(t){if(!this.overlay)return;if(t===null){this.overlay.setTitle(null);return}let e=this.posters[t];this.overlay.setTitle(e?e.title:null)}activate(t){let e=this.posters[t];e&&this.deps.bridge.navigate(`/media/${e.id}`)}};function ne(s){return!s||typeof s.id!="string"||!s.id||typeof s.posterUrl!="string"||!s.posterUrl?null:{id:s.id,title:typeof s.title=="string"?s.title:"",year:typeof s.year=="number"&&s.year>0?s.year:void 0,posterUrl:s.posterUrl}}var A=class{onReady(t){let e=(r,i)=>({locale:r!=null?r:"en",theme:i!=null?i:"dark"});typeof window!="undefined"&&window.evey?window.evey.onInit(r=>t(e(r==null?void 0:r.locale,r==null?void 0:r.theme))):t(e(navigator==null?void 0:navigator.language,void 0))}resize(t){var e;(e=window.evey)==null||e.resize(Math.round(t))}navigate(t){var e;(e=window.evey)==null||e.navigate(t)}async listMedia(){var e;let t=(e=window.evey)==null?void 0:e.media;if(!(t!=null&&t.list))return[];try{let r=await t.list({}),i=Array.isArray(r)?r:Array.isArray(r==null?void 0:r.items)?r.items:[],o=[];for(let n of i){let a=ne(n);a&&o.push(a)}return o}catch{return[]}}};function k(s,t,e,r){if(!(r>0))return 0;let i=(s+t*e)%r;return i<0?i+r:i}function z(s,t){if(!(t>0))return 0;let e=(s%t+t)%t/t;return e<.5?e*2:2-e*2}function X(s,t=1){return(s*2-1)*t}function V(s,t,e,r,i){let o=e>0?[-e,0,e]:[0],n=null;for(let a of s){let l=i.halfWidth*a.scale,c=i.halfHeight*a.scale;for(let u of o){let p=a.x-t+u;if(Math.abs(r.x-p)<=l&&Math.abs(r.y-a.y)<=c){(!n||a.z<n.z)&&(n=a);break}}}return n?n.index:null}var N=[[.55,.18,.16],[.16,.22,.42],[.3,.22,.1],[.14,.32,.28],[.35,.14,.3],[.22,.22,.22],[.45,.3,.1],[.12,.14,.24]];function K(s){let t=F(s!=null?s:"");return N[t%N.length]}var Y=`
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
`,j=`
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
`;function Z(s){let t=Math.max(1,Math.floor(s)),e=[],r=i=>e.indexOf(i);return{touch(i){let o=r(i);return o!==-1?(e.splice(o,1),e.push(i),null):(e.push(i),e.length>t?e.shift():null)},has(i){return r(i)!==-1},size(){return e.length},keys(){return e.slice()},remove(i){let o=r(i);o!==-1&&e.splice(o,1)}}}var ae=4,M=class{constructor(t,e){this.gl=t;this.textures=new Map;this.queue=[];this.activeLoads=0;this.disposed=!1;this.lru=Z(e)}reserve(t,e,r){let i=this.lru.touch(t);i!==null&&i!==t&&this.evict(i);let o=this.textures.get(t);return o||(o={tex:this.createPlaceholderTexture(r),loaded:!1},this.textures.set(t,o),this.queue.push({id:t,url:e})),o.tex}isLoaded(t){var e,r;return(r=(e=this.textures.get(t))==null?void 0:e.loaded)!=null?r:!1}pump(){for(;this.activeLoads<ae&&this.queue.length>0;){let t=this.queue.shift();if(!t)break;if(!this.textures.get(t.id))continue;this.activeLoads++;let r=new Image;r.onload=()=>{if(this.activeLoads--,this.disposed)return;let i=this.textures.get(t.id);if(!i)return;let o=this.gl;o.bindTexture(o.TEXTURE_2D,i.tex),o.pixelStorei(o.UNPACK_FLIP_Y_WEBGL,1),o.texImage2D(o.TEXTURE_2D,0,o.RGB,o.RGB,o.UNSIGNED_BYTE,r),o.pixelStorei(o.UNPACK_FLIP_Y_WEBGL,0),i.loaded=!0},r.onerror=()=>{this.activeLoads--},r.src=t.url}}dispose(){this.disposed=!0;for(let t of this.textures.values())this.gl.deleteTexture(t.tex);this.textures.clear(),this.queue.length=0}evict(t){let e=this.textures.get(t);e&&(this.gl.deleteTexture(e.tex),this.textures.delete(t))}createPlaceholderTexture(t){let e=this.gl,r=e.createTexture();if(!r)throw new Error("marquee: createTexture failed");e.bindTexture(e.TEXTURE_2D,r);let[i,o,n]=t;return e.texImage2D(e.TEXTURE_2D,0,e.RGB,1,1,0,e.RGB,e.UNSIGNED_BYTE,new Uint8Array([Math.round(i*255),Math.round(o*255),Math.round(n*255)])),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MIN_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_MAG_FILTER,e.LINEAR),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_S,e.CLAMP_TO_EDGE),e.texParameteri(e.TEXTURE_2D,e.TEXTURE_WRAP_T,e.CLAMP_TO_EDGE),r}};var L=v.colSpacing*.42,_=L/v.posterAspect,I=-.25,le=.05,ue=1.06,ce=.032,he=14e3,J=.22,g=null;function pe(){var s;if(g)return g;try{let t=document.createElement("canvas"),e=t.getContext("webgl2")||t.getContext("webgl");if(!e)return g={scale:.5,fps:24,maxDpr:1,posterCount:14,rows:2,reflections:!1,textureCapacity:16};let r=e.getParameter(e.MAX_TEXTURE_SIZE),i=(s=navigator.hardwareConcurrency)!=null?s:2;return"drawBuffers"in e&&r>=8192&&i>=4?g={scale:1,fps:30,maxDpr:1.5,posterCount:30,rows:3,reflections:!0,textureCapacity:32}:r>=4096?g={scale:.75,fps:30,maxDpr:1.25,posterCount:24,rows:3,reflections:!0,textureCapacity:28}:g={scale:.5,fps:24,maxDpr:1,posterCount:14,rows:2,reflections:!1,textureCapacity:16}}catch{return g={scale:.5,fps:24,maxDpr:1,posterCount:14,rows:2,reflections:!1,textureCapacity:16}}}function ee(s,t){if(!(t>0))return s;let e=(s%t+t)%t;return e>t/2&&(e-=t),e}function W(s,t,e){return s<t?t:s>e?e:s}var O=class{constructor(){this.host=null;this.canvas=null;this.gl=null;this.bgProgram=null;this.bgVbo=null;this.bgLoc={};this.posterProgram=null;this.quadVbo=null;this.posterLoc={};this.posterAttr=-1;this.textures=null;this.quality=pe();this.reducedMotion=typeof window!="undefined"&&window.matchMedia("(prefers-reduced-motion: reduce)").matches;this.slots=[];this.posters=[];this.period=1;this.scrollOffset=0;this.hoverIndex=null;this.hoverLift=0;this.spin=null;this.spinStartMs=0;this.lockedIndex=null;this.onLandedCb=null;this.raf=0;this.running=!1;this.inView=!0;this.lastTs=0;this.frameAcc=0;this.elapsedMs=0;this.lost=!1;this.activateCb=null;this.hoverCb=null;this.tileOffsets=[0,0,0];this.spotX=0;this.spotIntensity=0;this.spotLock=0;this.resizeObs=null;this.viewObs=null;this.onLost=t=>{t.preventDefault(),this.lost=!0};this.onRestored=()=>{this.lost=!1,this.initGL()};this.onPointerMove=t=>this.handlePointer(t,!1);this.onPointerUp=t=>this.handlePointer(t,!0);this.onPointerLeave=()=>this.setHover(null);this.loop=t=>{var a;if(!this.running)return;if(this.raf=requestAnimationFrame(this.loop),document.hidden||!this.inView||this.lost||!this.gl||!this.bgProgram||!this.posterProgram){this.lastTs=t;return}let e=Math.min((t-this.lastTs)/1e3,.25);this.lastTs=t,this.frameAcc+=e;let r=1/this.quality.fps;if(this.frameAcc<r)return;let i=Math.min(this.frameAcc,r*3);this.frameAcc=0;let o=this.reducedMotion?.25:1;this.elapsedMs+=i*o*1e3,this.spin!==null||this.lockedIndex!==null||(this.scrollOffset=k(this.scrollOffset,i*o,ce,this.period)),this.hoverLift+=((this.hoverIndex!==null?1:0)-this.hoverLift)*(1-Math.exp(-8*i)),(a=this.textures)==null||a.pump(),this.draw()}}getCapacity(){return{posterCount:this.quality.posterCount,rows:this.quality.rows}}mount(t){this.host=t;let e=document.createElement("canvas");e.style.cssText="position:absolute;inset:0;width:100%;height:100%;display:block;cursor:pointer;",t.appendChild(e),this.canvas=e,e.addEventListener("webglcontextlost",this.onLost,!1),e.addEventListener("webglcontextrestored",this.onRestored,!1),e.addEventListener("pointermove",this.onPointerMove),e.addEventListener("pointerup",this.onPointerUp),e.addEventListener("pointerleave",this.onPointerLeave),this.initGL(),this.resizeObs=new ResizeObserver(()=>this.resize()),this.resizeObs.observe(t),this.viewObs=new IntersectionObserver(r=>{var i,o;this.inView=(o=(i=r[0])==null?void 0:i.isIntersecting)!=null?o:!0}),this.viewObs.observe(t),this.resize(),this.running=!0,this.raf=requestAnimationFrame(this.loop)}setWall(t){this.slots=t.layout.slots,this.period=t.layout.period,this.posters=t.posters.map(e=>{let r=K(e.id),i=this.textures?this.textures.reserve(e.id,e.posterUrl,r):null;return{id:e.id,posterUrl:e.posterUrl,placeholder:r,tex:i}}),this.scrollOffset=0,this.hoverIndex=null,this.lockedIndex=null,this.spin=null}runRoulette(t,e){this.slots.length===0||t.steps.length===0||this.spin||(this.spin=t,this.spinStartMs=this.elapsedMs,this.lockedIndex=null,this.onLandedCb=e)}onActivate(t){this.activateCb=t}onHover(t){this.hoverCb=t}dispose(){var r,i,o,n;this.running=!1,cancelAnimationFrame(this.raf),(r=this.resizeObs)==null||r.disconnect(),(i=this.viewObs)==null||i.disconnect(),this.resizeObs=null,this.viewObs=null;let{gl:t,canvas:e}=this;e&&(e.removeEventListener("webglcontextlost",this.onLost),e.removeEventListener("webglcontextrestored",this.onRestored),e.removeEventListener("pointermove",this.onPointerMove),e.removeEventListener("pointerup",this.onPointerUp),e.removeEventListener("pointerleave",this.onPointerLeave)),(o=this.textures)==null||o.dispose(),this.textures=null,t&&(this.bgProgram&&t.deleteProgram(this.bgProgram),this.posterProgram&&t.deleteProgram(this.posterProgram),this.bgVbo&&t.deleteBuffer(this.bgVbo),this.quadVbo&&t.deleteBuffer(this.quadVbo),(n=t.getExtension("WEBGL_lose_context"))==null||n.loseContext()),e==null||e.remove(),this.gl=null,this.bgProgram=null,this.posterProgram=null,this.bgVbo=null,this.quadVbo=null,this.canvas=null,this.host=null}compile(t,e){let r=this.gl;if(!r)return null;let i=r.createShader(t);return i?(r.shaderSource(i,e),r.compileShader(i),r.getShaderParameter(i,r.COMPILE_STATUS)?i:(console.error("marquee shader:",r.getShaderInfoLog(i)),r.deleteShader(i),null)):null}link(t,e){let r=this.gl;if(!r)return null;let i=this.compile(r.VERTEX_SHADER,t),o=this.compile(r.FRAGMENT_SHADER,e);if(!i||!o)return null;let n=r.createProgram();return n?(r.attachShader(n,i),r.attachShader(n,o),r.linkProgram(n),r.deleteShader(i),r.deleteShader(o),r.getProgramParameter(n,r.LINK_STATUS)?n:(console.error("marquee link:",r.getProgramInfoLog(n)),r.deleteProgram(n),null)):null}initGL(){let t=this.canvas;if(!t)return;let e=t.getContext("webgl",{alpha:!0,premultipliedAlpha:!1,antialias:!1,depth:!1,stencil:!1,preserveDrawingBuffer:!1,powerPreference:"low-power"});if(e&&(this.gl=e,this.bgProgram=this.link(Y,Q),this.posterProgram=this.link(j,$),!(!this.bgProgram||!this.posterProgram))){this.bgVbo=e.createBuffer(),e.bindBuffer(e.ARRAY_BUFFER,this.bgVbo),e.bufferData(e.ARRAY_BUFFER,new Float32Array([-1,-1,3,-1,-1,3]),e.STATIC_DRAW);for(let r of["uRes","uTime","uSpotX","uSpotIntensity","uSpotWidth","uLock"])this.bgLoc[r]=e.getUniformLocation(this.bgProgram,r);this.quadVbo=e.createBuffer(),e.bindBuffer(e.ARRAY_BUFFER,this.quadVbo),e.bufferData(e.ARRAY_BUFFER,new Float32Array([-.5,-.5,.5,-.5,-.5,.5,.5,.5]),e.STATIC_DRAW),this.posterAttr=e.getAttribLocation(this.posterProgram,"aCorner");for(let r of["uCenter","uSize","uAspect","uTex","uPlaceholder","uLoaded","uDepthFade","uHighlight","uSpotLight","uAlpha"])this.posterLoc[r]=e.getUniformLocation(this.posterProgram,r);this.textures=new M(e,this.quality.textureCapacity),e.disable(e.DEPTH_TEST),e.clearColor(.02,.015,.02,1)}}resize(){let{host:t,canvas:e,gl:r}=this;if(!t||!e||!r)return;let i=Math.min(window.devicePixelRatio||1,this.quality.maxDpr)*this.quality.scale,o=Math.max(1,Math.round(t.clientWidth*i)),n=Math.max(1,Math.round(t.clientHeight*i));(e.width!==o||e.height!==n)&&(e.width=o,e.height=n,r.viewport(0,0,o,n))}aspect(){let t=this.canvas;return t&&t.height>0?t.width/t.height:1}setHover(t){var e;this.hoverIndex!==t&&(this.hoverIndex=t,(e=this.hoverCb)==null||e.call(this,t))}handlePointer(t,e){var p;let r=this.canvas;if(!r||this.slots.length===0)return;let i=r.getBoundingClientRect();if(i.width<=0||i.height<=0)return;let o=(t.clientX-i.left)/i.width*2-1,n=-((t.clientY-i.top)/i.height*2-1),a=this.aspect(),l=o*a,c=n-I,u=V(this.slots,this.scrollOffset,this.period,{x:l,y:c},{halfWidth:L,halfHeight:_});e?u!==null&&!this.spin&&((p=this.activateCb)==null||p.call(this,u)):this.setHover(u)}updateSpotlight(){let t=this.aspect();if(this.spin){let r=this.spin,i=this.elapsedMs-this.spinStartMs;if(i>=r.totalDurationMs){this.lockedIndex=r.winnerIndex,this.spin=null;let o=this.onLandedCb;this.onLandedCb=null,o==null||o(this.lockedIndex)}else{let o=0;for(let a=0;a<r.steps.length&&r.steps[a].atMs<=i;a++)o=a;let n=this.slots[r.steps[o].index];this.spotX=n?W(ee(n.x-this.scrollOffset,this.period)/t,-1,1):0,this.spotIntensity=1,this.spotLock=.4;return}}if(this.lockedIndex!==null){let r=this.slots[this.lockedIndex];this.spotX=r?W(ee(r.x-this.scrollOffset,this.period)/t,-1,1):0,this.spotIntensity=1,this.spotLock=1;return}let e=z(this.elapsedMs,he);this.spotX=X(e,1),this.spotIntensity=.55,this.spotLock=0}draw(){var n,a;let t=this.gl,e=this.canvas;if(!t||!e||!this.bgProgram||!this.posterProgram)return;this.updateSpotlight();let r=this.aspect();t.clear(t.COLOR_BUFFER_BIT),t.useProgram(this.bgProgram),t.bindBuffer(t.ARRAY_BUFFER,this.bgVbo);let i=t.getAttribLocation(this.bgProgram,"aPos");if(t.enableVertexAttribArray(i),t.vertexAttribPointer(i,2,t.FLOAT,!1,0,0),t.uniform2f(this.bgLoc.uRes,e.width,e.height),t.uniform1f(this.bgLoc.uTime,this.elapsedMs/1e3),t.uniform1f(this.bgLoc.uSpotX,this.spotX),t.uniform1f(this.bgLoc.uSpotIntensity,this.spotIntensity),t.uniform1f(this.bgLoc.uSpotWidth,J),t.uniform1f(this.bgLoc.uLock,this.spotLock),t.drawArrays(t.TRIANGLES,0,3),t.disableVertexAttribArray(i),this.posters.length===0)return;t.useProgram(this.posterProgram),t.bindBuffer(t.ARRAY_BUFFER,this.quadVbo),t.enableVertexAttribArray(this.posterAttr),t.vertexAttribPointer(this.posterAttr,2,t.FLOAT,!1,0,0),t.uniform1f(this.posterLoc.uAspect,r),t.uniform1i(this.posterLoc.uTex,0),t.activeTexture(t.TEXTURE0),t.uniform1f(this.posterLoc.uAlpha,1);let o=r+L;this.tileOffsets[0]=-this.period,this.tileOffsets[1]=0,this.tileOffsets[2]=this.period;for(let l=0;l<this.posters.length;l++){let c=this.slots[l],u=this.posters[l];if(!c||!u)continue;let p=u.tex,h=(a=(n=this.textures)==null?void 0:n.isLoaded(u.id))!=null?a:!1,d=l===this.hoverIndex||l===this.lockedIndex,m=d?1+(ue-1)*this.hoverLift:1;for(let f of this.tileOffsets){let b=c.x-this.scrollOffset+f;if(Math.abs(b)>o)continue;let w=c.y+I+(d?le*this.hoverLift:0),S=L*c.scale*m,R=_*c.scale*m,E=W(b/r,-1,1),y=W(1-Math.abs(E-this.spotX)/J,0,1);p&&t.bindTexture(t.TEXTURE_2D,p),t.uniform2f(this.posterLoc.uCenter,b,w),t.uniform2f(this.posterLoc.uSize,S,R),t.uniform3f(this.posterLoc.uPlaceholder,u.placeholder[0],u.placeholder[1],u.placeholder[2]),t.uniform1f(this.posterLoc.uLoaded,h?1:0),t.uniform1f(this.posterLoc.uDepthFade,c.z),t.uniform1f(this.posterLoc.uHighlight,d?this.hoverLift:0),t.uniform1f(this.posterLoc.uSpotLight,y),t.drawArrays(t.TRIANGLE_STRIP,0,4)}}this.quality.reflections&&this.drawReflections(o),t.disableVertexAttribArray(this.posterAttr)}drawReflections(t){var i,o;let e=this.gl;if(!e)return;let r=I-_;e.enable(e.BLEND),e.blendFunc(e.SRC_ALPHA,e.ONE_MINUS_SRC_ALPHA),e.uniform1f(this.posterLoc.uAlpha,.14),e.uniform1f(this.posterLoc.uHighlight,0),e.uniform1f(this.posterLoc.uSpotLight,.6);for(let n=0;n<this.posters.length;n++){let a=this.slots[n],l=this.posters[n];if(!a||a.row!==0||!l)continue;let c=l.tex,u=(o=(i=this.textures)==null?void 0:i.isLoaded(l.id))!=null?o:!1,p=L*a.scale,h=_*a.scale,d=2*r-(a.y+I);for(let m of this.tileOffsets){let f=a.x-this.scrollOffset+m;Math.abs(f)>t||(c&&e.bindTexture(e.TEXTURE_2D,c),e.uniform2f(this.posterLoc.uCenter,f,d),e.uniform2f(this.posterLoc.uSize,p,h),e.uniform3f(this.posterLoc.uPlaceholder,l.placeholder[0],l.placeholder[1],l.placeholder[2]),e.uniform1f(this.posterLoc.uLoaded,u?1:0),e.uniform1f(this.posterLoc.uDepthFade,a.z),e.drawArrays(e.TRIANGLE_STRIP,0,4))}}e.disable(e.BLEND)}};new C({renderer:new O,bridge:new A}).start();})();
