"use strict";(()=>{var c={aurore:{id:"aurore",anchors:[[.24,.16,.52],[.62,.18,.56],[.86,.56,.22],[.82,.36,.46]],vignette:[.3,.14,.1]},neon:{id:"neon",anchors:[[.05,.82,.92],[.92,.14,.72],[.52,.16,.94],[.14,.34,.96]],vignette:[.08,.02,.16]},braise:{id:"braise",anchors:[[.32,.05,.05],[.74,.14,.08],[.93,.46,.12],[.86,.62,.22]],vignette:[.16,.03,.02]},ocean:{id:"ocean",anchors:[[.02,.24,.34],[.05,.24,.62],[.1,.62,.78],[.18,.72,.58]],vignette:[.02,.1,.14]}},oe=Object.keys(c),d="aurore";function H(s){return s==="aurore"||s==="neon"||s==="braise"||s==="ocean"}function h(s){return H(s)?s:d}var O=`
.lum-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: #05030a;
}
.lum-scene { position: absolute; inset: 0; cursor: default; }
`;function A(s,e,i){return Math.min(i,Math.max(e,s))}function E(s){let e=document.createElement("style");e.textContent=O,document.head.appendChild(e);let i=document.createElement("div");i.className="lum-root";let t=document.createElement("div");t.className="lum-scene",i.appendChild(t),s.appendChild(i);let r=null,n=l=>{let p=t.getBoundingClientRect(),y=p.width/2||1,P=p.height/2||1,N=A((l.clientX-p.left-y)/y,-1,1),G=A((l.clientY-p.top-P)/P,-1,1);r==null||r(N,G)},a=()=>r==null?void 0:r(0,0);return t.addEventListener("pointermove",n),t.addEventListener("pointerleave",a),{canvasHost:t,onPointer(l){r=l},dispose(){t.removeEventListener("pointermove",n),t.removeEventListener("pointerleave",a),i.remove(),e.remove()}}}var z=60*1e3,k="appearance.palette",W="appearance.intensity",U="appearance.speed",Y=.2,q=1,X=.3,Q=2;function x(s,e,i){return Math.min(i,Math.max(e,s))}var f=class{constructor(e){this.deps=e;this.overlay=null;this.init=null;this.palette="aurore";this.intensity=.7;this.speed=1;this.refreshTimer=0;this.disposed=!1;this.booted=!1;this.onPageHide=()=>this.dispose();this.onResize=()=>this.requestHeight();this.onVisible=()=>{document.hidden||this.push()}}start(){this.deps.bridge.onReady(e=>void this.boot(e))}dispose(){var e;this.disposed||(this.disposed=!0,clearInterval(this.refreshTimer),window.removeEventListener("pagehide",this.onPageHide),window.removeEventListener("resize",this.onResize),document.removeEventListener("visibilitychange",this.onVisible),this.deps.renderer.dispose(),(e=this.overlay)==null||e.dispose(),this.overlay=null)}async boot(e){this.booted||(this.booted=!0,this.init=e,this.palette=e.palette,this.intensity=e.intensity,this.speed=e.speed,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent",document.body.style.overflow="hidden",this.overlay=E(document.body),this.deps.renderer.mount(this.overlay.canvasHost),this.overlay.onPointer((i,t)=>this.deps.renderer.setPointer(i,t)),this.requestHeight(),window.addEventListener("resize",this.onResize),window.addEventListener("pagehide",this.onPageHide),document.addEventListener("visibilitychange",this.onVisible),this.push(),await this.loadSettings(),!this.disposed&&(this.push(),this.refreshTimer=window.setInterval(()=>void this.refreshAndPush(),z)))}async refreshAndPush(){await this.loadSettings(),!this.disposed&&this.push()}async loadSettings(){let e=this.init;if(e)try{let[i,t,r]=await Promise.all([this.deps.bridge.getPref(k),this.deps.bridge.getPref(W),this.deps.bridge.getPref(U)]);this.palette=i!==null?h(i):e.palette;let n=t!==null?Number(t):NaN;this.intensity=Number.isFinite(n)?x(n,Y,q):e.intensity;let a=r!==null?Number(r):NaN;this.speed=Number.isFinite(a)?x(a,X,Q):e.speed}catch{}}requestHeight(){let e=window.innerWidth||document.documentElement.clientWidth||800,i=e<420?260:e<700?340:420;this.deps.bridge.resize(i)}push(){this.disposed||!this.overlay||this.deps.renderer.setTarget({paletteId:this.palette,intensity:this.intensity,speedMul:this.speed})}};var K=.7,j=1,Z=.2,J=1,$=.3,ee=2;function T(s,e,i,t){let r=typeof s=="number"?s:typeof s=="string"?Number(s):NaN;return Number.isFinite(r)?Math.min(t,Math.max(i,r)):e}var m=class{onReady(e){let i=(t,r,n)=>({palette:h(t==null?void 0:t.palette),intensity:T(t==null?void 0:t.intensity,K,Z,J),speed:T(t==null?void 0:t.speed,j,$,ee),locale:r!=null?r:"en",theme:n!=null?n:"dark"});typeof window!="undefined"&&window.evey?window.evey.onInit(t=>e(i(t==null?void 0:t.config,t==null?void 0:t.locale,t==null?void 0:t.theme))):e(i(void 0,navigator==null?void 0:navigator.language,void 0))}resize(e){var i;(i=window.evey)==null||i.resize(Math.round(e))}async getPref(e){var t;let i=(t=window.evey)==null?void 0:t.prefs;if(!(i!=null&&i.get))return null;try{let r=await i.get(e);return r==null?null:typeof r=="string"?r:r.found===!1?null:typeof r.value=="string"?r.value:null}catch{return null}}async setPref(e,i){var r;let t=(r=window.evey)==null?void 0:r.prefs;if(t!=null&&t.set)try{await t.set(e,i)}catch{}}toast(e,i){var t,r;(r=(t=window.evey)==null?void 0:t.toast)==null||r.call(t,e,i)}};function te(s){return s*s*(3-2*s)}function ie(s,e,i){return[s[0]+(e[0]-s[0])*i,s[1]+(e[1]-s[1])*i,s[2]+(e[2]-s[2])*i]}function re(s,e){return[s[0]*e,s[1]*e,s[2]*e]}function ne(s,e){return[s[0]+(1-s[0])*e,s[1]+(1-s[1])*e,s[2]+(1-s[2])*e]}function g(s,e){let i=s.length,r=(e%1+1)%1*i,n=Math.floor(r)%i,a=(n+1)%i,l=r-Math.floor(r);return ie(s[n],s[a],te(l))}function S(s,e){let i=e/36e4,t=g(s.anchors,i),r=g(s.anchors,i+.37),n=re(g(s.anchors,i+-.19),.32),a=ne(t,.55);return{veilA:t,veilB:r,veilC:n,caustic:a,vignette:s.vignette}}var R={low:14,mid:26,high:40},se=R.high;function w(s){return R[s]}function u(s){let e=Math.sin(s)*43758.5453123;return e-Math.floor(e)}function L(s,e){let i=Math.max(0,Math.min(Math.floor(s),se)),t=new Array(i);for(let r=0;r<i;r++){let n=e+r*12.9898;t[r]={baseX:u(n*1.1)*2-1,baseY:u(n*2.3+7.1)*2-1,depth:u(n*3.7+4.2),size:.4+u(n*5.9+1.3)*.6,phase:u(n*7.7+2.6)*Math.PI*2,speed:.15+u(n*9.1+.4)*.35}}return t}function _(){return{x:0,y:0}}function I(s,e,i,t){let r=1-Math.exp(-1.6*Math.max(0,t));s.x+=(e-s.x)*r,s.y+=(i-s.y)*r}function C(){return{time:0,drift:0,causticPhase:0,vignettePulse:0}}function V(s,e,i){let t=Math.max(0,e)*i;s.time=(s.time+t)%4096,s.drift=(s.drift+t*.1)%512,s.causticPhase=(s.causticPhase+t*.42)%4096,s.vignettePulse=(s.vignettePulse+t*.06)%4096}var B=`
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`,M=`
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec2  uRes;
uniform float uTime;
uniform float uDrift;
uniform float uCausticPhase;
uniform float uVignettePulse;
uniform vec2  uPointer;      // eased, -1..1
uniform float uIntensity;    // 0..1 user setting

uniform vec3 uVeilA;
uniform vec3 uVeilB;
uniform vec3 uVeilC;
uniform vec3 uCaustic;
uniform vec3 uVignette;

uniform float uExtraLayer; // 0/1, quality-gated third veil
uniform float uCaustics;   // 0/1, quality-gated liquid-light pass

varying vec2 vUv;

float hash21(vec2 p) {
  p = fract(p * vec2(123.34, 345.45));
  p += dot(p, p + 34.345);
  return fract(p.x * p.y);
}

float vnoise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  float a = hash21(i);
  float b = hash21(i + vec2(1.0, 0.0));
  float c = hash21(i + vec2(0.0, 1.0));
  float d = hash21(i + vec2(1.0, 1.0));
  return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

float fbm(vec2 p) {
  float v = 0.0;
  float a = 0.5;
  mat2 m = mat2(1.6, 1.2, -1.2, 1.6);
  for (int i = 0; i < 4; i++) {
    v += a * vnoise(p);
    p = m * p;
    a *= 0.5;
  }
  return v;
}

// One drifting, domain-warped "curtain" of light: a soft FBM band stretched
// mostly vertically, warped by a second noise field so it ripples instead of
// scrolling as a rigid stripe.
float curtain(vec2 uv, float freqX, float freqY, float driftMul, float warpAmt, float seed) {
  vec2 warp = vec2(fbm(uv * 1.3 + seed), fbm(uv * 1.7 - seed)) * warpAmt;
  vec2 p = vec2(uv.x * freqX + uDrift * driftMul, uv.y * freqY - uTime * 0.02 + seed) + warp;
  float n = fbm(p);
  return smoothstep(0.30, 0.95, n);
}

void main() {
  float aspect = uRes.x / max(uRes.y, 1.0);
  vec2 uv = vUv;
  vec2 p = (uv - 0.5) * vec2(aspect, 1.0) * 2.0;

  // 1. Deep background wash: the darkest palette anchor, softly brighter
  // toward the center so the veils have somewhere to bloom from.
  vec3 col = uVeilC * (0.85 + 0.25 * exp(-dot(p, p) * 0.5));

  // 2. Veil A: the primary curtain.
  float rayA = curtain(uv, 2.2, 1.0, 0.045, 0.6, 1.7);
  col += uVeilA * rayA * 0.55 * uIntensity;

  // 3. Veil B: a second curtain at a different frequency/direction, phase-
  // shifted in the palette (see paletteDrift.ts) so it never carries the
  // same color as A - this is what reads as "crossing" rather than moving
  // together.
  vec2 uvB = vec2(uv.x * 0.82 + uv.y * 0.35, uv.y * 0.9 - uv.x * 0.20);
  float rayB = curtain(uvB, 1.6, 1.3, -0.07, 0.5, 9.3);
  col += uVeilB * rayB * 0.48 * uIntensity;

  // 4. Veil C (extra layer, quality-gated): a finer, slower third curtain
  // adding parallax depth on the higher tiers only.
  if (uExtraLayer > 0.5) {
    vec2 uvC = vec2(uv.x * 1.1 - uv.y * 0.5, uv.y * 2.4 + uv.x * 0.15);
    float rayC = curtain(uvC, 3.4, 1.8, 0.02, 0.35, 27.1);
    col += mix(uVeilA, uVeilB, 0.5) * rayC * 0.24 * uIntensity;
  }

  // 5. Caustics (quality-gated): a classic interference-of-sines liquid-
  // light pattern, additive, subtle - like the bottom of a lit pool.
  if (uCaustics > 0.5) {
    vec2 cp = uv * 5.0 + vec2(sin(uCausticPhase * 0.30), cos(uCausticPhase * 0.23));
    float c1 = abs(sin(cp.x * 3.0 + cos(cp.y * 2.0 + uCausticPhase * 0.5)));
    float c2 = abs(sin(cp.y * 3.3 - sin(cp.x * 2.4 - uCausticPhase * 0.4)));
    float caustic = pow(c1 * c2, 3.0);
    col += uCaustic * caustic * 0.35 * uIntensity;
  }

  // 6. Lazy pointer-reactive glow: a soft highlight that drifts toward the
  // (already-eased) pointer position, never snapping.
  vec2 ptr = uPointer * vec2(aspect, 1.0);
  float glow = exp(-dot(p - ptr, p - ptr) * 2.2) * 0.16 * uIntensity;
  col += mix(uVeilA, uVeilB, 0.5) * glow;

  // 7. Warm vignette, breathing slowly.
  float edge = length(p);
  float vigMask = smoothstep(0.55, 1.55, edge) * (0.78 + 0.14 * sin(uVignettePulse * 0.6));
  col = mix(col, col * 0.35 + uVignette * 0.30, vigMask);

  // 8. Film grain: cheap per-pixel dither, time-varying so it never reads as
  // a static texture (and keeps the deep gradients above from banding).
  float grain = hash21(gl_FragCoord.xy + uTime * 71.0) - 0.5;
  col += grain * 0.030;

  gl_FragColor = vec4(clamp(col, 0.0, 1.0), 1.0);
}
`,D=`
attribute vec2  aBase;
attribute float aDepth;
attribute float aSize;
attribute float aPhase;
attribute float aSpeed;

uniform float uTime;
uniform vec2  uPointer;
uniform float uIntensity;

varying float vDepth;
varying float vTwinkle;

void main() {
  float t = uTime * aSpeed + aPhase;
  // Gentle floating drift, larger for the far/small particles so the field
  // reads as a soft haze rather than a grid.
  vec2 drift = vec2(sin(t * 0.7), cos(t * 0.5)) * (0.05 + 0.05 * (1.0 - aDepth));
  // Parallax toward the (already lazily-eased) pointer, stronger for near
  // particles - a shallow depth-of-field cue with zero extra CPU work.
  vec2 parallax = uPointer * aDepth * 0.10;
  vec2 pos = aBase + drift + parallax;

  gl_Position = vec4(pos, 0.0, 1.0);
  gl_PointSize = aSize * (10.0 + 14.0 * aDepth) * (0.6 + 0.4 * uIntensity);

  vDepth = aDepth;
  vTwinkle = 0.5 + 0.5 * sin(t * 1.3 + aPhase * 2.0);
}
`,F=`
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec3  uColor;
uniform float uIntensity;

varying float vDepth;
varying float vTwinkle;

void main() {
  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float r = length(c);
  float bokeh = exp(-r * r * 3.2);
  float alpha = bokeh * (0.25 + 0.55 * vDepth) * (0.5 + 0.5 * vTwinkle) * uIntensity;
  gl_FragColor = vec4(uColor * bokeh, alpha);
}
`;var o=null;function ae(){var s;if(o)return o;try{let e=document.createElement("canvas"),i=e.getContext("webgl2")||e.getContext("webgl");if(!i)return o={scale:.5,fps:24,maxDpr:1,extraLayer:!1,caustics:!1,particleTier:"low"};let t=i.getParameter(i.MAX_TEXTURE_SIZE),r=(s=navigator.hardwareConcurrency)!=null?s:2;return"drawBuffers"in i&&t>=8192&&r>=4?o={scale:1,fps:30,maxDpr:1.5,extraLayer:!0,caustics:!0,particleTier:"high"}:t>=4096?o={scale:.75,fps:30,maxDpr:1.25,extraLayer:!0,caustics:!1,particleTier:"mid"}:o={scale:.5,fps:24,maxDpr:1,extraLayer:!1,caustics:!1,particleTier:"low"}}catch{return o={scale:.5,fps:24,maxDpr:1,extraLayer:!1,caustics:!1,particleTier:"low"}}}var b=6,v=class{constructor(){this.host=null;this.canvas=null;this.gl=null;this.veilProgram=null;this.veilVbo=null;this.veilLoc={};this.particleProgram=null;this.particleVbo=null;this.particleLoc={};this.particleAttr={};this.particleSeeds=[];this.particleCount=0;this.quality=ae();this.reducedMotion=typeof window!="undefined"&&window.matchMedia("(prefers-reduced-motion: reduce)").matches;this.presetDef=c[d];this.intensity=.7;this.speedMul=1;this.paletteElapsedMs=0;this.anim=C();this.pointer=_();this.pointerTargetX=0;this.pointerTargetY=0;this.fieldSeed=Math.random()*1e3;this.raf=0;this.running=!1;this.inView=!0;this.lastTs=0;this.frameAcc=0;this.lost=!1;this.resizeObs=null;this.viewObs=null;this.onLost=e=>{e.preventDefault(),this.lost=!0};this.onRestored=()=>{this.lost=!1,this.initGL()};this.loop=e=>{if(!this.running)return;if(this.raf=requestAnimationFrame(this.loop),document.hidden||!this.inView||this.lost||!this.gl){this.lastTs=e;return}let i=Math.min((e-this.lastTs)/1e3,.25);this.lastTs=e,this.frameAcc+=i;let t=1/this.quality.fps;if(this.frameAcc<t)return;let r=Math.min(this.frameAcc,t*3);this.frameAcc=0;let n=this.reducedMotion?.25:1,a=this.speedMul*n;V(this.anim,r,a),I(this.pointer,this.pointerTargetX,this.pointerTargetY,r),this.paletteElapsedMs+=r*1e3*a,this.draw()}}mount(e){this.host=e;let i=document.createElement("canvas");i.style.cssText="position:absolute;inset:0;width:100%;height:100%;display:block;",e.appendChild(i),this.canvas=i,i.addEventListener("webglcontextlost",this.onLost,!1),i.addEventListener("webglcontextrestored",this.onRestored,!1),this.initGL(),this.resizeObs=new ResizeObserver(()=>this.resize()),this.resizeObs.observe(e),this.viewObs=new IntersectionObserver(t=>{var r,n;this.inView=(n=(r=t[0])==null?void 0:r.isIntersecting)!=null?n:!0}),this.viewObs.observe(e),this.resize(),this.running=!0,this.raf=requestAnimationFrame(this.loop)}setTarget(e){var i;this.presetDef=(i=c[e.paletteId])!=null?i:c[d],this.intensity=e.intensity,this.speedMul=e.speedMul}setPointer(e,i){this.pointerTargetX=e,this.pointerTargetY=i}dispose(){var t,r,n;this.running=!1,cancelAnimationFrame(this.raf),(t=this.resizeObs)==null||t.disconnect(),(r=this.viewObs)==null||r.disconnect(),this.resizeObs=null,this.viewObs=null;let{gl:e,canvas:i}=this;i&&(i.removeEventListener("webglcontextlost",this.onLost),i.removeEventListener("webglcontextrestored",this.onRestored)),e&&(this.veilProgram&&e.deleteProgram(this.veilProgram),this.particleProgram&&e.deleteProgram(this.particleProgram),this.veilVbo&&e.deleteBuffer(this.veilVbo),this.particleVbo&&e.deleteBuffer(this.particleVbo),(n=e.getExtension("WEBGL_lose_context"))==null||n.loseContext()),i==null||i.remove(),this.gl=null,this.veilProgram=null,this.particleProgram=null,this.veilVbo=null,this.particleVbo=null,this.canvas=null,this.host=null}compile(e,i){let t=this.gl;if(!t)return null;let r=t.createShader(e);return r?(t.shaderSource(r,i),t.compileShader(r),t.getShaderParameter(r,t.COMPILE_STATUS)?r:(console.error("lumina shader:",t.getShaderInfoLog(r)),t.deleteShader(r),null)):null}link(e,i){let t=this.gl;if(!t)return null;let r=this.compile(t.VERTEX_SHADER,e),n=this.compile(t.FRAGMENT_SHADER,i);if(!r||!n)return null;let a=t.createProgram();return a?(t.attachShader(a,r),t.attachShader(a,n),t.linkProgram(a),t.deleteShader(r),t.deleteShader(n),t.getProgramParameter(a,t.LINK_STATUS)?a:(console.error("lumina link:",t.getProgramInfoLog(a)),t.deleteProgram(a),null)):null}initGL(){let e=this.canvas;if(!e)return;let i=e.getContext("webgl",{alpha:!0,premultipliedAlpha:!1,antialias:!1,depth:!1,stencil:!1,preserveDrawingBuffer:!1,powerPreference:"low-power"});if(i&&(this.gl=i,this.veilProgram=this.link(B,M),this.particleProgram=this.link(D,F),!(!this.veilProgram||!this.particleProgram))){this.veilVbo=i.createBuffer(),i.bindBuffer(i.ARRAY_BUFFER,this.veilVbo),i.bufferData(i.ARRAY_BUFFER,new Float32Array([-1,-1,3,-1,-1,3]),i.STATIC_DRAW);for(let t of["uRes","uTime","uDrift","uCausticPhase","uVignettePulse","uPointer","uIntensity","uVeilA","uVeilB","uVeilC","uCaustic","uVignette","uExtraLayer","uCaustics"])this.veilLoc[t]=i.getUniformLocation(this.veilProgram,t);this.particleVbo=i.createBuffer();for(let t of["uTime","uPointer","uIntensity","uColor"])this.particleLoc[t]=i.getUniformLocation(this.particleProgram,t);for(let t of["aBase","aDepth","aSize","aPhase","aSpeed"])this.particleAttr[t]=i.getAttribLocation(this.particleProgram,t);this.uploadParticles(),i.disable(i.DEPTH_TEST),i.clearColor(0,0,0,1)}}uploadParticles(){let e=this.gl;if(!e||!this.particleVbo)return;let i=w(this.quality.particleTier);this.particleSeeds=L(i,this.fieldSeed),this.particleCount=this.particleSeeds.length;let t=new Float32Array(this.particleCount*b);for(let r=0;r<this.particleCount;r++){let n=this.particleSeeds[r],a=r*b;t[a]=n.baseX,t[a+1]=n.baseY,t[a+2]=n.depth,t[a+3]=n.size,t[a+4]=n.phase,t[a+5]=n.speed}e.bindBuffer(e.ARRAY_BUFFER,this.particleVbo),e.bufferData(e.ARRAY_BUFFER,t,e.STATIC_DRAW)}resize(){let{host:e,canvas:i,gl:t}=this;if(!e||!i||!t)return;let r=Math.min(window.devicePixelRatio||1,this.quality.maxDpr)*this.quality.scale,n=Math.max(1,Math.round(e.clientWidth*r)),a=Math.max(1,Math.round(e.clientHeight*r));(i.width!==n||i.height!==a)&&(i.width=n,i.height=a,t.viewport(0,0,n,a))}draw(){let e=this.gl,i=this.canvas;if(!e||!i)return;let t=S(this.presetDef,this.paletteElapsedMs);if(e.disable(e.BLEND),e.clear(e.COLOR_BUFFER_BIT),this.veilProgram&&this.veilVbo){e.useProgram(this.veilProgram),e.bindBuffer(e.ARRAY_BUFFER,this.veilVbo);let r=e.getAttribLocation(this.veilProgram,"aPos");e.enableVertexAttribArray(r),e.vertexAttribPointer(r,2,e.FLOAT,!1,0,0);let n=this.veilLoc;e.uniform2f(n.uRes,i.width,i.height),e.uniform1f(n.uTime,this.anim.time),e.uniform1f(n.uDrift,this.anim.drift),e.uniform1f(n.uCausticPhase,this.anim.causticPhase),e.uniform1f(n.uVignettePulse,this.anim.vignettePulse),e.uniform2f(n.uPointer,this.pointer.x,this.pointer.y),e.uniform1f(n.uIntensity,this.intensity),e.uniform3f(n.uVeilA,t.veilA[0],t.veilA[1],t.veilA[2]),e.uniform3f(n.uVeilB,t.veilB[0],t.veilB[1],t.veilB[2]),e.uniform3f(n.uVeilC,t.veilC[0],t.veilC[1],t.veilC[2]),e.uniform3f(n.uCaustic,t.caustic[0],t.caustic[1],t.caustic[2]),e.uniform3f(n.uVignette,t.vignette[0],t.vignette[1],t.vignette[2]),e.uniform1f(n.uExtraLayer,this.quality.extraLayer?1:0),e.uniform1f(n.uCaustics,this.quality.caustics?1:0),e.drawArrays(e.TRIANGLES,0,3),e.disableVertexAttribArray(r)}if(this.particleProgram&&this.particleVbo&&this.particleCount>0){e.enable(e.BLEND),e.blendFunc(e.SRC_ALPHA,e.ONE),e.useProgram(this.particleProgram),e.bindBuffer(e.ARRAY_BUFFER,this.particleVbo);let r=b*4,n=this.particleAttr;e.enableVertexAttribArray(n.aBase),e.vertexAttribPointer(n.aBase,2,e.FLOAT,!1,r,0),e.enableVertexAttribArray(n.aDepth),e.vertexAttribPointer(n.aDepth,1,e.FLOAT,!1,r,8),e.enableVertexAttribArray(n.aSize),e.vertexAttribPointer(n.aSize,1,e.FLOAT,!1,r,12),e.enableVertexAttribArray(n.aPhase),e.vertexAttribPointer(n.aPhase,1,e.FLOAT,!1,r,16),e.enableVertexAttribArray(n.aSpeed),e.vertexAttribPointer(n.aSpeed,1,e.FLOAT,!1,r,20);let a=this.particleLoc;e.uniform1f(a.uTime,this.anim.time),e.uniform2f(a.uPointer,this.pointer.x,this.pointer.y),e.uniform1f(a.uIntensity,this.intensity),e.uniform3f(a.uColor,t.caustic[0],t.caustic[1],t.caustic[2]),e.drawArrays(e.POINTS,0,this.particleCount),e.disableVertexAttribArray(n.aBase),e.disableVertexAttribArray(n.aDepth),e.disableVertexAttribArray(n.aSize),e.disableVertexAttribArray(n.aPhase),e.disableVertexAttribArray(n.aSpeed)}}};new f({renderer:new v,bridge:new m}).start();})();
