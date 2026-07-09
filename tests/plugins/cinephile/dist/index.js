"use strict";(()=>{function P(n){let e=new Set,t=[];for(let i of n)!i||!i.id||!i.title||i.type!=="movie"&&i.type!=="series"||!i.year||i.year<1870||i.year>2100||e.has(i.id)||(e.add(i.id),t.push(i));return t}function G(n){return Math.floor(n/10)*10}function y(n){let e=((n+9)%100).toString().padStart(2,"0");return`${n}-${e}`}function x(n){let e=new Map;for(let t of n){if(!t.year)continue;let i=G(t.year),r=e.get(i);r?r.push(t):e.set(i,[t])}return Array.from(e.entries()).map(([t,i])=>({decade:t,items:i})).sort((t,i)=>t.decade-i.decade)}function S(n){let e=n>>>0;return function(){e|=0,e=e+1831565813|0;let i=Math.imul(e^e>>>15,1|e);return i=i+Math.imul(i^i>>>7,61|i)^i,((i^i>>>14)>>>0)/4294967296}}function R(n){let e=2166136261;for(let t=0;t<n.length;t++)e^=n.charCodeAt(t),e=Math.imul(e,16777619);return e>>>0}function L(n,e){for(let t=n.length-1;t>0;t--){let i=Math.floor(e()*(t+1)),r=n[t];n[t]=n[i],n[i]=r}return n}function N(n,e,t){let i=Math.max(0,Math.floor(e));if(i===0||n.length===0)return[];let r=S(t),o=L(n.slice(),r);return o.slice(0,Math.min(i,o.length))}var H=.07,$=.24,V=.5,X=.16;function j(n){let e=Math.max(0,Math.min(1,n));return H+($-H)*Math.sqrt(e)}function Q(n){return R(n)%1e3/1e3}function D(n,e){let t=Math.max(1,Math.floor(e.maxNodes)),i=Math.max(0,Math.floor(e.representativePerNode)),r=n.reduce((c,u)=>c+u.items.length,0);if(r===0)return[];let o=n.filter(c=>c.items.length>0).slice().sort((c,u)=>c.decade-u.decade),a=o,d=null;if(o.length>t){let c=o.slice().sort((p,f)=>f.items.length-p.items.length),u=new Set(c.slice(0,t-1).map(p=>p.decade));a=o.filter(p=>u.has(p.decade));let h=o.filter(p=>!u.has(p.decade)).flatMap(p=>p.items);h.length>0&&(d=h)}let l=a.map(c=>({id:`decade-${c.decade}`,decade:c.decade,items:c.items}));d&&l.push({id:"other",decade:null,items:d});let s=l.length,v=[];for(let c=0;c<s;c++){let u=l[c],m=u.items.length,h=m/r,p=c/s*Math.PI*2-Math.PI/2,f=V+Q(u.id)*X;v.push({id:u.id,decade:u.decade,count:m,share:h,x:Math.cos(p)*f,y:Math.sin(p)*f,radius:j(h),representative:N(u.items,i,R(u.id))})}return v}function A(n){var l;let e=n.filter(s=>s.items.length>0);if(e.length===0)return[];let t=e.slice().sort((s,v)=>s.decade-v.decade),i=t[0].decade,r=t[t.length-1].decade,o=new Map(t.map(s=>[s.decade,s.items.length])),a=Math.max(...t.map(s=>s.items.length)),d=[];for(let s=i;s<=r;s+=10){let v=(l=o.get(s))!=null?l:0;d.push({decade:s,count:v,heightFraction:a>0?v/a:0})}return d}function z(n){let e=0,t=0,i=null,r=null;for(let l of n)l.type==="movie"?e++:l.type==="series"&&t++,l.year&&((i===null||l.year<i)&&(i=l.year),(r===null||l.year>r)&&(r=l.year));let o=null,a=0;for(let l of x(n))l.items.length>a&&(a=l.items.length,o=l.decade);let d=n.length;return{totalCount:d,dominantDecade:o,dominantDecadeShare:d>0?a/d:0,movieCount:e,seriesCount:t,oldestYear:i,newestYear:r}}var U={profileHeading:{en:"Your cinephile profile",fr:"Ton profil de cin\xE9phile",es:"Tu perfil de cin\xE9filo",de:"Dein Cinephilen-Profil"},statTotal:{en:"Titles",fr:"Titres",es:"T\xEDtulos",de:"Titel"},statDominantDecade:{en:"Favorite decade",fr:"D\xE9cennie pr\xE9f\xE9r\xE9e",es:"D\xE9cada favorita",de:"Lieblingsjahrzehnt"},statMovies:{en:"Movies",fr:"Films",es:"Pel\xEDculas",de:"Filme"},statSeries:{en:"Series",fr:"S\xE9ries",es:"Series",de:"Serien"},statSpan:{en:"Span",fr:"\xC9tendue",es:"Rango",de:"Zeitraum"},otherDecade:{en:"Other",fr:"Autres",es:"Otros",de:"Weitere"},titlesSuffix:{en:"titles",fr:"titres",es:"t\xEDtulos",de:"Titel"},empty:{en:"Add movies or shows to your library to reveal your taste galaxy.",fr:"Ajoute des films ou s\xE9ries \xE0 ta biblioth\xE8que pour r\xE9v\xE9ler ta galaxie de go\xFBts.",es:"A\xF1ade pel\xEDculas o series a tu biblioteca para revelar tu galaxia de gustos.",de:"F\xFCge Filme oder Serien hinzu, um deine Geschmacksgalaxie zu enth\xFCllen."}};function g(n,e){var r;let t=U[n];if(!t)return n;let i=(e!=null?e:"en").slice(0,2).toLowerCase();return(r=t[i])!=null?r:t.en}var O=`
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
`,W=O.replace(/cqi/g,"vw");function B(n){var c,u;let e=document.createElement("style"),t=((u=(c=window.CSS)==null?void 0:c.supports)==null?void 0:u.call(c,"width: 1cqi"))===!0;e.textContent=t?`.cp-root { container-type: inline-size; }
${O}`:W,document.head.appendChild(e);let i=document.createElement("div");i.className="cp-root";let r=document.createElement("div");r.className="cp-scene",i.appendChild(r);let o=document.createElement("div");o.className="cp-tooltip",r.appendChild(o);let a=document.createElement("div");a.className="cp-profile";let d=document.createElement("div");d.className="cp-profile-heading",a.appendChild(d);let l=document.createElement("div");a.appendChild(l),r.appendChild(a);let s=document.createElement("div");s.className="cp-timeline",i.appendChild(s);let v=document.createElement("div");return v.className="cp-empty",v.style.display="none",i.appendChild(v),n.appendChild(i),{canvasHost:r,setTooltip(m){if(!m){o.classList.remove("visible");return}o.textContent=m,o.classList.add("visible")},setProfile(m,h){if(!m||m.length===0){a.classList.remove("visible");return}d.textContent=h,l.replaceChildren();for(let p of m){let f=document.createElement("div");f.className="cp-profile-row";let T=document.createElement("span");T.textContent=p.label;let I=document.createElement("span");I.className="cp-profile-value",I.textContent=p.value,f.appendChild(T),f.appendChild(I),l.appendChild(f)}a.classList.add("visible")},setTimeline(m,h){s.replaceChildren();for(let p of m){let f=document.createElement("div");f.className="cp-timeline-bar",f.style.height=`${Math.max(4,p.heightFraction*100)}%`,s.appendChild(f)}if(h){let p=document.createElement("div");p.className="cp-timeline-edge start",p.textContent=h.start,s.appendChild(p);let f=document.createElement("div");f.className="cp-timeline-edge end",f.textContent=h.end,s.appendChild(f)}},setEmptyMessage(m){v.textContent=m!=null?m:"",v.style.display=m?"flex":"none"},dispose(){i.remove(),e.remove()}}}var K=30*60*1e3,J=4,w=class{constructor(e){this.deps=e;this.overlay=null;this.init=null;this.nodes=[];this.refreshTimer=0;this.disposed=!1;this.booted=!1;this.onPageHide=()=>this.dispose();this.onResize=()=>this.requestHeight();this.onVisible=()=>{document.hidden||this.loadLibrary()}}start(){this.deps.bridge.onReady(e=>void this.boot(e))}dispose(){var e;this.disposed||(this.disposed=!0,clearInterval(this.refreshTimer),window.removeEventListener("pagehide",this.onPageHide),window.removeEventListener("resize",this.onResize),document.removeEventListener("visibilitychange",this.onVisible),this.deps.renderer.dispose(),(e=this.overlay)==null||e.dispose(),this.overlay=null)}async boot(e){this.booted||(this.booted=!0,this.init=e,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent",document.body.style.overflow="hidden",this.overlay=B(document.body),this.deps.renderer.mount(this.overlay.canvasHost),this.deps.renderer.onActivate(t=>this.activate(t)),this.deps.renderer.onHover(t=>this.hover(t)),this.requestHeight(),window.addEventListener("resize",this.onResize),window.addEventListener("pagehide",this.onPageHide),document.addEventListener("visibilitychange",this.onVisible),await this.loadLibrary(),!this.disposed&&(this.refreshTimer=window.setInterval(()=>void this.loadLibrary(),K)))}requestHeight(){let e=window.innerWidth||document.documentElement.clientWidth||800,t=e<420?380:e<700?460:520;this.deps.bridge.resize(t)}async loadLibrary(){var d,l;if(this.disposed||!this.overlay)return;let e=await this.deps.bridge.listMedia();if(this.disposed)return;let t=P(e);if(t.length===0){this.nodes=[],this.deps.renderer.setScene({nodes:[]}),this.overlay.setProfile(null,""),this.overlay.setTimeline([],null),this.overlay.setTooltip(null),this.overlay.setEmptyMessage(g("empty",(d=this.init)==null?void 0:d.locale));return}this.overlay.setEmptyMessage(null);let i=x(t),r=this.deps.renderer.getCapacity();this.nodes=D(i,{maxNodes:r.maxNodes,representativePerNode:J}),this.deps.renderer.setScene({nodes:this.nodes}),this.overlay.setProfile(this.buildProfileRows(t),g("profileHeading",(l=this.init)==null?void 0:l.locale));let o=A(i),a=o.length>1?{start:y(o[0].decade),end:y(o[o.length-1].decade)}:null;this.overlay.setTimeline(o,a)}buildProfileRows(e){var o;let t=(o=this.init)==null?void 0:o.locale,i=z(e),r=[{label:g("statTotal",t),value:String(i.totalCount)}];return i.dominantDecade!==null&&r.push({label:g("statDominantDecade",t),value:y(i.dominantDecade)}),r.push({label:g("statMovies",t),value:String(i.movieCount)}),r.push({label:g("statSeries",t),value:String(i.seriesCount)}),i.oldestYear!==null&&i.newestYear!==null&&r.push({label:g("statSpan",t),value:`${i.oldestYear} - ${i.newestYear}`}),r}hover(e){var d,l;if(!this.overlay)return;if(e===null){this.overlay.setTooltip(null);return}let t=this.nodes[e];if(!t){this.overlay.setTooltip(null);return}let i=t.decade===null?g("otherDecade",(d=this.init)==null?void 0:d.locale):y(t.decade),r=g("titlesSuffix",(l=this.init)==null?void 0:l.locale),o=t.representative.slice(0,3).map(s=>s.title).join(", "),a=[`${i} \xB7 ${t.count} ${r}`];o&&a.push(o),this.overlay.setTooltip(a.join(" \xB7 "))}activate(e){let t=this.nodes[e],i=t==null?void 0:t.representative[0];i&&this.deps.bridge.navigate(`/media/${i.id}`)}};function Z(n){return n==="MEDIA_ITEM_TYPE_MOVIE"?"movie":n==="MEDIA_ITEM_TYPE_SERIES"?"series":"other"}function ee(n){return!n||typeof n.id!="string"||!n.id||typeof n.title!="string"||!n.title?null:{id:n.id,title:n.title,year:typeof n.year=="number"&&n.year>0?n.year:void 0,posterUrl:typeof n.posterUrl=="string"&&n.posterUrl?n.posterUrl:void 0,type:Z(n.type)}}var C=class{onReady(e){let t=(i,r)=>({locale:i!=null?i:"en",theme:r!=null?r:"dark"});typeof window!="undefined"&&window.evey?window.evey.onInit(i=>e(t(i==null?void 0:i.locale,i==null?void 0:i.theme))):e(t(navigator==null?void 0:navigator.language,void 0))}resize(e){var t;(t=window.evey)==null||t.resize(Math.round(e))}navigate(e){var t;(t=window.evey)==null||t.navigate(e)}async listMedia(){var t;let e=(t=window.evey)==null?void 0:t.media;if(!(e!=null&&e.list))return[];try{let i=await e.list({}),r=Array.isArray(i)?i:Array.isArray(i==null?void 0:i.items)?i.items:[],o=[];for(let a of r){let d=ee(a);d&&o.push(d)}return o}catch{return[]}}};function k(n,e){let t=null,i=1/0;for(let r=0;r<n.length;r++){let o=n[r],a=e.x-o.x,d=e.y-o.y;a*a+d*d<=o.radius*o.radius&&o.radius<i&&(i=o.radius,t=r)}return t}function q(n){switch(n){case"low":return 6;case"mid":return 9;default:return 14}}function _(n){return n!=="low"}var b=null;function te(){var n;if(b)return b;try{let e=typeof navigator!="undefined"&&(n=navigator.hardwareConcurrency)!=null?n:2,t=typeof window!="undefined"&&window.devicePixelRatio||1;return e>=4&&t<=2?b="high":e>=2?b="mid":b="low"}catch{return b="low"}}var F=.42,ie=.1,ne=1.08,re=5e-4,Y,M=class{constructor(){this.host=null;this.canvas=null;this.ctx=null;this.tier=te();this.glow=_(this.tier);this.reducedMotion=typeof window!="undefined"&&((Y=window.matchMedia)==null?void 0:Y.call(window,"(prefers-reduced-motion: reduce)").matches)===!0;this.nodes=[];this.state=[];this.raf=0;this.running=!1;this.inView=!0;this.lastTs=0;this.time=0;this.resizeObs=null;this.viewObs=null;this.activateCb=null;this.hoverCb=null;this.hoveredIndex=null;this.onPointerMove=e=>this.handlePointer(e.clientX,e.clientY,!1);this.onPointerLeave=()=>this.setHovered(null);this.onClick=e=>this.handlePointer(e.clientX,e.clientY,!0);this.loop=e=>{if(!this.running)return;if(this.raf=requestAnimationFrame(this.loop),document.hidden||!this.inView||!this.ctx){this.lastTs=e;return}let t=Math.min((e-this.lastTs)/1e3,.25);this.lastTs=e,this.time+=this.reducedMotion?t*.25:t,this.advance(t),this.draw()}}getCapacity(){return{maxNodes:q(this.tier)}}mount(e){this.host=e;let t=document.createElement("canvas");t.style.cssText="position:absolute;inset:0;width:100%;height:100%;display:block;cursor:pointer;",e.appendChild(t),this.canvas=t,this.ctx=t.getContext("2d"),t.addEventListener("pointermove",this.onPointerMove),t.addEventListener("pointerleave",this.onPointerLeave),t.addEventListener("click",this.onClick),this.resizeObs=new ResizeObserver(()=>this.resize()),this.resizeObs.observe(e),this.viewObs=new IntersectionObserver(i=>{var r,o;this.inView=(o=(r=i[0])==null?void 0:r.isIntersecting)!=null?o:!0}),this.viewObs.observe(e),this.resize(),this.running=!0,this.raf=requestAnimationFrame(this.loop)}setScene(e){this.nodes=e.nodes;let t=this.state,i=new Array(e.nodes.length);for(let r=0;r<e.nodes.length;r++){let o=e.nodes[r],a=t[r],d=oe(o.decade);i[r]={x:a?a.x:0,y:a?a.y:0,radius:a?a.radius:0,targetX:o.x,targetY:o.y,targetRadius:o.radius,opacity:a?a.opacity:0,phase:r*.61803398875%1,hovered:!1,colorInner:`hsla(${d}, 90%, 78%, 0.95)`,colorOuter:`hsla(${d}, 78%, 42%, 0.88)`,glowColor:`hsla(${d}, 85%, 70%, 0.55)`,gradient:null,gradientPx:NaN,gradientPy:NaN,gradientR:NaN}}this.state=i,this.hoveredIndex=null}onActivate(e){this.activateCb=e}onHover(e){this.hoverCb=e}dispose(){var t,i;this.running=!1,cancelAnimationFrame(this.raf),(t=this.resizeObs)==null||t.disconnect(),(i=this.viewObs)==null||i.disconnect(),this.resizeObs=null,this.viewObs=null;let e=this.canvas;e&&(e.removeEventListener("pointermove",this.onPointerMove),e.removeEventListener("pointerleave",this.onPointerLeave),e.removeEventListener("click",this.onClick),e.remove()),this.canvas=null,this.ctx=null,this.host=null,this.nodes=[],this.state=[],this.activateCb=null,this.hoverCb=null,this.hoveredIndex=null}resize(){let{host:e,canvas:t}=this;if(!e||!t)return;let i=Math.min(typeof window!="undefined"&&window.devicePixelRatio||1,2),r=Math.max(1,Math.round(e.clientWidth*i)),o=Math.max(1,Math.round(e.clientHeight*i));(t.width!==r||t.height!==o)&&(t.width=r,t.height=o)}handlePointer(e,t,i){var u;let r=this.canvas;if(!r||this.state.length===0){if(i)return;this.setHovered(null);return}let o=r.getBoundingClientRect(),a=o.width||1,d=o.height||1,l=Math.min(a,d)*F;if(l<=0)return;let s=(e-o.left-a/2)/l,v=(t-o.top-d/2)/l,c=k(this.state,{x:s,y:v});if(i){c!==null&&((u=this.activateCb)==null||u.call(this,c));return}this.setHovered(c)}setHovered(e){var t;e!==this.hoveredIndex&&(this.hoveredIndex!==null&&this.state[this.hoveredIndex]&&(this.state[this.hoveredIndex].hovered=!1),this.hoveredIndex=e,e!==null&&this.state[e]&&(this.state[e].hovered=!0),(t=this.hoverCb)==null||t.call(this,e))}advance(e){let t=Math.min(1,ie*(e*60));for(let i=0;i<this.state.length;i++){let r=this.state[i];r.x=E(r.x,r.targetX,t),r.y=E(r.y,r.targetY,t),r.radius=E(r.radius,r.targetRadius,t),r.opacity=E(r.opacity,1,t)}}draw(){let e=this.ctx,t=this.canvas;if(!e||!t)return;let i=t.width,r=t.height;e.clearRect(0,0,i,r);let o=Math.min(i,r)*F,a=i/2,d=r/2;for(let l=0;l<this.state.length;l++){let s=this.state[l];if(!this.nodes[l]||s.opacity<=.001)continue;let c=a+s.x*o,u=d+s.y*o,h=Math.max(1,s.radius*o)*(s.hovered?ne:1);if(e.globalAlpha=s.opacity,this.glow){let p=this.reducedMotion?1:.82+Math.sin(this.time*1.4+s.phase*Math.PI*2)*.18;e.shadowBlur=h*.9*p,e.shadowColor=s.glowColor}else e.shadowBlur=0;if(!s.gradient||s.gradientPx!==c||s.gradientPy!==u||s.gradientR!==h){let p=e.createRadialGradient(c,u,0,c,u,h);p.addColorStop(0,s.colorInner),p.addColorStop(1,s.colorOuter),s.gradient=p,s.gradientPx=c,s.gradientPy=u,s.gradientR=h}e.fillStyle=s.gradient,e.beginPath(),e.arc(c,u,h,0,Math.PI*2),e.fill()}e.globalAlpha=1,e.shadowBlur=0}};function E(n,e,t){let i=n+(e-n)*t;return Math.abs(e-i)<re?e:i}function oe(n){return n===null?205:((270-Math.round((n-1900)/10)*10)%360+360)%360}new w({renderer:new M,bridge:new C}).start();})();
