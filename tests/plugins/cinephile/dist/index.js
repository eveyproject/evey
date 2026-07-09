"use strict";(()=>{function R(r){let e=new Set,t=[];for(let i of r)!i||!i.id||!i.title||i.type!=="movie"&&i.type!=="series"||!i.year||i.year<1870||i.year>2100||e.has(i.id)||(e.add(i.id),t.push(i));return t}function Y(r){return Math.floor(r/10)*10}function x(r){let e=((r+9)%100).toString().padStart(2,"0");return`${r}-${e}`}function w(r){let e=new Map;for(let t of r){if(!t.year)continue;let i=Y(t.year),n=e.get(i);n?n.push(t):e.set(i,[t])}return Array.from(e.entries()).map(([t,i])=>({decade:t,items:i})).sort((t,i)=>t.decade-i.decade)}function S(r){let e=r>>>0;return function(){e|=0,e=e+1831565813|0;let i=Math.imul(e^e>>>15,1|e);return i=i+Math.imul(i^i>>>7,61|i)^i,((i^i>>>14)>>>0)/4294967296}}function T(r){let e=2166136261;for(let t=0;t<r.length;t++)e^=r.charCodeAt(t),e=Math.imul(e,16777619);return e>>>0}function P(r,e){for(let t=r.length-1;t>0;t--){let i=Math.floor(e()*(t+1)),n=r[t];r[t]=r[i],r[i]=n}return r}function L(r,e,t){let i=Math.max(0,Math.floor(e));if(i===0||r.length===0)return[];let n=S(t),o=P(r.slice(),n);return o.slice(0,Math.min(i,o.length))}var H=.07,G=.24,$=.5,V=.16;function X(r){let e=Math.max(0,Math.min(1,r));return H+(G-H)*Math.sqrt(e)}function j(r){return T(r)%1e3/1e3}function D(r,e){let t=Math.max(1,Math.floor(e.maxNodes)),i=Math.max(0,Math.floor(e.representativePerNode)),n=r.reduce((c,u)=>c+u.items.length,0);if(n===0)return[];let o=r.filter(c=>c.items.length>0).slice().sort((c,u)=>c.decade-u.decade),s=o,d=null;if(o.length>t){let c=o.slice().sort((p,m)=>m.items.length-p.items.length),u=new Set(c.slice(0,t-1).map(p=>p.decade));s=o.filter(p=>u.has(p.decade));let v=o.filter(p=>!u.has(p.decade)).flatMap(p=>p.items);v.length>0&&(d=v)}let a=s.map(c=>({id:`decade-${c.decade}`,decade:c.decade,items:c.items}));d&&a.push({id:"other",decade:null,items:d});let l=a.length,f=[];for(let c=0;c<l;c++){let u=a[c],h=u.items.length,v=h/n,p=c/l*Math.PI*2-Math.PI/2,m=$+j(u.id)*V;f.push({id:u.id,decade:u.decade,count:h,share:v,x:Math.cos(p)*m,y:Math.sin(p)*m,radius:X(v),representative:L(u.items,i,T(u.id))})}return f}function A(r){var a;let e=r.filter(l=>l.items.length>0);if(e.length===0)return[];let t=e.slice().sort((l,f)=>l.decade-f.decade),i=t[0].decade,n=t[t.length-1].decade,o=new Map(t.map(l=>[l.decade,l.items.length])),s=Math.max(...t.map(l=>l.items.length)),d=[];for(let l=i;l<=n;l+=10){let f=(a=o.get(l))!=null?a:0;d.push({decade:l,count:f,heightFraction:s>0?f/s:0})}return d}function N(r){let e=0,t=0,i=null,n=null;for(let a of r)a.type==="movie"?e++:a.type==="series"&&t++,a.year&&((i===null||a.year<i)&&(i=a.year),(n===null||a.year>n)&&(n=a.year));let o=null,s=0;for(let a of w(r))a.items.length>s&&(s=a.items.length,o=a.decade);let d=r.length;return{totalCount:d,dominantDecade:o,dominantDecadeShare:d>0?s/d:0,movieCount:e,seriesCount:t,oldestYear:i,newestYear:n}}var Q={profileHeading:{en:"Your cinephile profile",fr:"Ton profil de cin\xE9phile",es:"Tu perfil de cin\xE9filo",de:"Dein Cinephilen-Profil"},statTotal:{en:"Titles",fr:"Titres",es:"T\xEDtulos",de:"Titel"},statDominantDecade:{en:"Favorite decade",fr:"D\xE9cennie pr\xE9f\xE9r\xE9e",es:"D\xE9cada favorita",de:"Lieblingsjahrzehnt"},statMovies:{en:"Movies",fr:"Films",es:"Pel\xEDculas",de:"Filme"},statSeries:{en:"Series",fr:"S\xE9ries",es:"Series",de:"Serien"},statSpan:{en:"Span",fr:"\xC9tendue",es:"Rango",de:"Zeitraum"},otherDecade:{en:"Other",fr:"Autres",es:"Otros",de:"Weitere"},titlesSuffix:{en:"titles",fr:"titres",es:"t\xEDtulos",de:"Titel"},empty:{en:"Add movies or shows to your library to reveal your taste galaxy.",fr:"Ajoute des films ou s\xE9ries \xE0 ta biblioth\xE8que pour r\xE9v\xE9ler ta galaxie de go\xFBts.",es:"A\xF1ade pel\xEDculas o series a tu biblioteca para revelar tu galaxia de gustos.",de:"F\xFCge Filme oder Serien hinzu, um deine Geschmacksgalaxie zu enth\xFCllen."}};function g(r,e){var n;let t=Q[r];if(!t)return r;let i=(e!=null?e:"en").slice(0,2).toLowerCase();return(n=t[i])!=null?n:t.en}var z=`
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
`,U=z.replace(/cqi/g,"vw");function B(r){var c,u;let e=document.createElement("style"),t=((u=(c=window.CSS)==null?void 0:c.supports)==null?void 0:u.call(c,"width: 1cqi"))===!0;e.textContent=t?`.cp-root { container-type: inline-size; }
${z}`:U,document.head.appendChild(e);let i=document.createElement("div");i.className="cp-root";let n=document.createElement("div");n.className="cp-scene",i.appendChild(n);let o=document.createElement("div");o.className="cp-tooltip",n.appendChild(o);let s=document.createElement("div");s.className="cp-profile";let d=document.createElement("div");d.className="cp-profile-heading",s.appendChild(d);let a=document.createElement("div");s.appendChild(a),n.appendChild(s);let l=document.createElement("div");l.className="cp-timeline",i.appendChild(l);let f=document.createElement("div");return f.className="cp-empty",f.style.display="none",i.appendChild(f),r.appendChild(i),{canvasHost:n,setTooltip(h){if(!h){o.classList.remove("visible");return}o.textContent=h,o.classList.add("visible")},setProfile(h,v){if(!h||h.length===0){s.classList.remove("visible");return}d.textContent=v,a.replaceChildren();for(let p of h){let m=document.createElement("div");m.className="cp-profile-row";let y=document.createElement("span");y.textContent=p.label;let I=document.createElement("span");I.className="cp-profile-value",I.textContent=p.value,m.appendChild(y),m.appendChild(I),a.appendChild(m)}s.classList.add("visible")},setTimeline(h,v){l.replaceChildren();for(let p of h){let m=document.createElement("div");m.className="cp-timeline-bar",m.style.height=`${Math.max(4,p.heightFraction*100)}%`,l.appendChild(m)}if(v){let p=document.createElement("div");p.className="cp-timeline-edge start",p.textContent=v.start,l.appendChild(p);let m=document.createElement("div");m.className="cp-timeline-edge end",m.textContent=v.end,l.appendChild(m)}},setEmptyMessage(h){f.textContent=h!=null?h:"",f.style.display=h?"flex":"none"},dispose(){i.remove(),e.remove()}}}var W=30*60*1e3,K=4,C=class{constructor(e){this.deps=e;this.overlay=null;this.init=null;this.nodes=[];this.refreshTimer=0;this.disposed=!1;this.booted=!1;this.onPageHide=()=>this.dispose();this.onResize=()=>this.requestHeight();this.onVisible=()=>{document.hidden||this.loadLibrary()}}start(){this.deps.bridge.onReady(e=>void this.boot(e))}dispose(){var e;this.disposed||(this.disposed=!0,clearInterval(this.refreshTimer),window.removeEventListener("pagehide",this.onPageHide),window.removeEventListener("resize",this.onResize),document.removeEventListener("visibilitychange",this.onVisible),this.deps.renderer.dispose(),(e=this.overlay)==null||e.dispose(),this.overlay=null)}async boot(e){this.booted||(this.booted=!0,this.init=e,document.documentElement.style.background="transparent",document.body.style.margin="0",document.body.style.background="transparent",document.body.style.overflow="hidden",this.overlay=B(document.body),this.deps.renderer.mount(this.overlay.canvasHost),this.deps.renderer.onActivate(t=>this.activate(t)),this.deps.renderer.onHover(t=>this.hover(t)),this.requestHeight(),window.addEventListener("resize",this.onResize),window.addEventListener("pagehide",this.onPageHide),document.addEventListener("visibilitychange",this.onVisible),await this.loadLibrary(),!this.disposed&&(this.refreshTimer=window.setInterval(()=>void this.loadLibrary(),W)))}requestHeight(){let e=window.innerWidth||document.documentElement.clientWidth||800,t=e<420?380:e<700?460:520;this.deps.bridge.resize(t)}async loadLibrary(){var d,a;if(this.disposed||!this.overlay)return;let e=await this.deps.bridge.listMedia();if(this.disposed)return;let t=R(e);if(t.length===0){this.nodes=[],this.deps.renderer.setScene({nodes:[]}),this.overlay.setProfile(null,""),this.overlay.setTimeline([],null),this.overlay.setTooltip(null),this.overlay.setEmptyMessage(g("empty",(d=this.init)==null?void 0:d.locale));return}this.overlay.setEmptyMessage(null);let i=w(t),n=this.deps.renderer.getCapacity();this.nodes=D(i,{maxNodes:n.maxNodes,representativePerNode:K}),this.deps.renderer.setScene({nodes:this.nodes}),this.overlay.setProfile(this.buildProfileRows(t),g("profileHeading",(a=this.init)==null?void 0:a.locale));let o=A(i),s=o.length>1?{start:x(o[0].decade),end:x(o[o.length-1].decade)}:null;this.overlay.setTimeline(o,s)}buildProfileRows(e){var o;let t=(o=this.init)==null?void 0:o.locale,i=N(e),n=[{label:g("statTotal",t),value:String(i.totalCount)}];return i.dominantDecade!==null&&n.push({label:g("statDominantDecade",t),value:x(i.dominantDecade)}),n.push({label:g("statMovies",t),value:String(i.movieCount)}),n.push({label:g("statSeries",t),value:String(i.seriesCount)}),i.oldestYear!==null&&i.newestYear!==null&&n.push({label:g("statSpan",t),value:`${i.oldestYear} - ${i.newestYear}`}),n}hover(e){var d,a;if(!this.overlay)return;if(e===null){this.overlay.setTooltip(null);return}let t=this.nodes[e];if(!t){this.overlay.setTooltip(null);return}let i=t.decade===null?g("otherDecade",(d=this.init)==null?void 0:d.locale):x(t.decade),n=g("titlesSuffix",(a=this.init)==null?void 0:a.locale),o=t.representative.slice(0,3).map(l=>l.title).join(", "),s=[`${i} \xB7 ${t.count} ${n}`];o&&s.push(o),this.overlay.setTooltip(s.join(" \xB7 "))}activate(e){let t=this.nodes[e],i=t==null?void 0:t.representative[0];i&&this.deps.bridge.navigate(`/media/${i.id}`)}};function J(r){return r==="MEDIA_ITEM_TYPE_MOVIE"?"movie":r==="MEDIA_ITEM_TYPE_SERIES"?"series":"other"}function Z(r){return!r||typeof r.id!="string"||!r.id||typeof r.title!="string"||!r.title?null:{id:r.id,title:r.title,year:typeof r.year=="number"&&r.year>0?r.year:void 0,posterUrl:typeof r.posterUrl=="string"&&r.posterUrl?r.posterUrl:void 0,type:J(r.type)}}var E=class{onReady(e){let t=(i,n)=>({locale:i!=null?i:"en",theme:n!=null?n:"dark"});typeof window!="undefined"&&window.evey?window.evey.onInit(i=>e(t(i==null?void 0:i.locale,i==null?void 0:i.theme))):e(t(navigator==null?void 0:navigator.language,void 0))}resize(e){var t;(t=window.evey)==null||t.resize(Math.round(e))}navigate(e){var t;(t=window.evey)==null||t.navigate(e)}async listMedia(){var t;let e=(t=window.evey)==null?void 0:t.media;if(!(e!=null&&e.list))return[];try{let i=await e.list({}),n=Array.isArray(i)?i:Array.isArray(i==null?void 0:i.items)?i.items:[],o=[];for(let s of n){let d=Z(s);d&&o.push(d)}return o}catch{return[]}}};function k(r,e){let t=null,i=1/0;for(let n=0;n<r.length;n++){let o=r[n],s=e.x-o.x,d=e.y-o.y;s*s+d*d<=o.radius*o.radius&&o.radius<i&&(i=o.radius,t=n)}return t}function O(r){switch(r){case"low":return 6;case"mid":return 9;default:return 14}}function q(r){return r!=="low"}var b=null;function ee(){var r;if(b)return b;try{let e=typeof navigator!="undefined"&&(r=navigator.hardwareConcurrency)!=null?r:2,t=typeof window!="undefined"&&window.devicePixelRatio||1;return e>=4&&t<=2?b="high":e>=2?b="mid":b="low"}catch{return b="low"}}var _=.42,te=.1,ie=1.08,F,M=class{constructor(){this.host=null;this.canvas=null;this.ctx=null;this.tier=ee();this.glow=q(this.tier);this.reducedMotion=typeof window!="undefined"&&((F=window.matchMedia)==null?void 0:F.call(window,"(prefers-reduced-motion: reduce)").matches)===!0;this.nodes=[];this.state=[];this.raf=0;this.running=!1;this.inView=!0;this.lastTs=0;this.time=0;this.resizeObs=null;this.viewObs=null;this.activateCb=null;this.hoverCb=null;this.hoveredIndex=null;this.onPointerMove=e=>this.handlePointer(e.clientX,e.clientY,!1);this.onPointerLeave=()=>this.setHovered(null);this.onClick=e=>this.handlePointer(e.clientX,e.clientY,!0);this.loop=e=>{if(!this.running)return;if(this.raf=requestAnimationFrame(this.loop),document.hidden||!this.inView||!this.ctx){this.lastTs=e;return}let t=Math.min((e-this.lastTs)/1e3,.25);this.lastTs=e,this.time+=this.reducedMotion?t*.25:t,this.advance(t),this.draw()}}getCapacity(){return{maxNodes:O(this.tier)}}mount(e){this.host=e;let t=document.createElement("canvas");t.style.cssText="position:absolute;inset:0;width:100%;height:100%;display:block;cursor:pointer;",e.appendChild(t),this.canvas=t,this.ctx=t.getContext("2d"),t.addEventListener("pointermove",this.onPointerMove),t.addEventListener("pointerleave",this.onPointerLeave),t.addEventListener("click",this.onClick),this.resizeObs=new ResizeObserver(()=>this.resize()),this.resizeObs.observe(e),this.viewObs=new IntersectionObserver(i=>{var n,o;this.inView=(o=(n=i[0])==null?void 0:n.isIntersecting)!=null?o:!0}),this.viewObs.observe(e),this.resize(),this.running=!0,this.raf=requestAnimationFrame(this.loop)}setScene(e){this.nodes=e.nodes;let t=this.state,i=new Array(e.nodes.length);for(let n=0;n<e.nodes.length;n++){let o=e.nodes[n],s=t[n];i[n]={x:s?s.x:0,y:s?s.y:0,radius:s?s.radius:0,targetX:o.x,targetY:o.y,targetRadius:o.radius,opacity:s?s.opacity:0,phase:n*.61803398875%1,hovered:!1}}this.state=i,this.hoveredIndex=null}onActivate(e){this.activateCb=e}onHover(e){this.hoverCb=e}dispose(){var t,i;this.running=!1,cancelAnimationFrame(this.raf),(t=this.resizeObs)==null||t.disconnect(),(i=this.viewObs)==null||i.disconnect(),this.resizeObs=null,this.viewObs=null;let e=this.canvas;e&&(e.removeEventListener("pointermove",this.onPointerMove),e.removeEventListener("pointerleave",this.onPointerLeave),e.removeEventListener("click",this.onClick),e.remove()),this.canvas=null,this.ctx=null,this.host=null,this.nodes=[],this.state=[],this.activateCb=null,this.hoverCb=null,this.hoveredIndex=null}resize(){let{host:e,canvas:t}=this;if(!e||!t)return;let i=Math.min(typeof window!="undefined"&&window.devicePixelRatio||1,2),n=Math.max(1,Math.round(e.clientWidth*i)),o=Math.max(1,Math.round(e.clientHeight*i));(t.width!==n||t.height!==o)&&(t.width=n,t.height=o)}handlePointer(e,t,i){var u;let n=this.canvas;if(!n||this.state.length===0){if(i)return;this.setHovered(null);return}let o=n.getBoundingClientRect(),s=o.width||1,d=o.height||1,a=Math.min(s,d)*_;if(a<=0)return;let l=(e-o.left-s/2)/a,f=(t-o.top-d/2)/a,c=k(this.state,{x:l,y:f});if(i){c!==null&&((u=this.activateCb)==null||u.call(this,c));return}this.setHovered(c)}setHovered(e){var t;e!==this.hoveredIndex&&(this.hoveredIndex!==null&&this.state[this.hoveredIndex]&&(this.state[this.hoveredIndex].hovered=!1),this.hoveredIndex=e,e!==null&&this.state[e]&&(this.state[e].hovered=!0),(t=this.hoverCb)==null||t.call(this,e))}advance(e){let t=Math.min(1,te*(e*60));for(let i=0;i<this.state.length;i++){let n=this.state[i];n.x+=(n.targetX-n.x)*t,n.y+=(n.targetY-n.y)*t,n.radius+=(n.targetRadius-n.radius)*t,n.opacity+=(1-n.opacity)*t}}draw(){let e=this.ctx,t=this.canvas;if(!e||!t)return;let i=t.width,n=t.height;e.clearRect(0,0,i,n);let o=Math.min(i,n)*_,s=i/2,d=n/2;for(let a=0;a<this.state.length;a++){let l=this.state[a],f=this.nodes[a];if(!f||l.opacity<=.001)continue;let c=s+l.x*o,u=d+l.y*o,h=Math.max(1,l.radius*o),v=this.reducedMotion?0:Math.sin(this.time*1.4+l.phase*Math.PI*2)*.06,p=h*(1+v)*(l.hovered?ie:1),m=ne(f.decade);e.globalAlpha=l.opacity,this.glow?(e.shadowBlur=p*.9,e.shadowColor=`hsla(${m}, 85%, 70%, 0.55)`):e.shadowBlur=0;let y=e.createRadialGradient(c,u,0,c,u,p);y.addColorStop(0,`hsla(${m}, 90%, 78%, 0.95)`),y.addColorStop(1,`hsla(${m}, 78%, 42%, 0.88)`),e.fillStyle=y,e.beginPath(),e.arc(c,u,p,0,Math.PI*2),e.fill()}e.globalAlpha=1,e.shadowBlur=0}};function ne(r){return r===null?205:((270-Math.round((r-1900)/10)*10)%360+360)%360}new C({renderer:new M,bridge:new E}).start();})();
