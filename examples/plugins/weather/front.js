// Weather front module — vanilla WebGL fragment-shader sky + overlay text.
// No framework: single fullscreen triangle, ~80 lines of GLSL drives the
// whole visual (gradient sky, fbm clouds, sun bloom, rain, snow, storm).
// CSP: only fetches to api.open-meteo.com + geocoding-api.open-meteo.com
// (declared in plugin.yaml external_fetch). No unsafe-eval needed —
// shaders compile via gl.shaderSource which is GPU text, not JS eval.

(function () {
  // --- weather code → scene profile (sky colors + active effects) ---
  // Mapping based on Open-Meteo WMO codes.
  function profileFor(code) {
    if (code === 0) return { kind: 0, label: "Clear",        topA: [0.36,0.66,0.94], topB: [0.96,0.78,0.55], cloud: 0.0, sun: 1.0 };
    if (code === 1 || code === 2) return { kind: 1, label: "Partly cloudy", topA: [0.42,0.66,0.88], topB: [0.85,0.85,0.92], cloud: 0.45, sun: 0.85 };
    if (code === 3) return { kind: 1, label: "Overcast",     topA: [0.50,0.55,0.62], topB: [0.78,0.80,0.82], cloud: 0.85, sun: 0.10 };
    if (code >= 45 && code <= 48) return { kind: 5, label: "Fog",      topA: [0.62,0.64,0.66], topB: [0.85,0.86,0.86], cloud: 1.0, sun: 0.0 };
    if (code >= 51 && code <= 67) return { kind: 2, label: "Rain",     topA: [0.30,0.36,0.46], topB: [0.55,0.60,0.66], cloud: 0.95, sun: 0.0 };
    if (code >= 71 && code <= 77) return { kind: 3, label: "Snow",     topA: [0.62,0.66,0.74], topB: [0.86,0.90,0.95], cloud: 0.85, sun: 0.0 };
    if (code >= 80 && code <= 82) return { kind: 2, label: "Showers",  topA: [0.36,0.42,0.52], topB: [0.60,0.65,0.70], cloud: 0.95, sun: 0.0 };
    if (code >= 85 && code <= 86) return { kind: 3, label: "Snow showers", topA: [0.66,0.70,0.78], topB: [0.90,0.92,0.96], cloud: 0.85, sun: 0.0 };
    if (code >= 95) return { kind: 4, label: "Thunderstorm", topA: [0.18,0.18,0.26], topB: [0.30,0.30,0.40], cloud: 1.0, sun: 0.0 };
    return { kind: 1, label: "Unknown", topA: [0.45,0.55,0.65], topB: [0.80,0.84,0.88], cloud: 0.5, sun: 0.0 };
  }

  // --- shaders ---
  var VS = "attribute vec2 a;void main(){gl_Position=vec4(a,0.0,1.0);}";
  var FS = [
    "precision mediump float;",
    "uniform vec2 u_res;",
    "uniform float u_time;",
    "uniform vec3 u_skyTop;",
    "uniform vec3 u_skyBot;",
    "uniform float u_cloud;", // 0..1 cloud coverage
    "uniform float u_sun;",   // 0..1 sun brightness
    "uniform int u_kind;",    // 2=rain,3=snow,4=storm,5=fog
    // 2D hash + value noise + 3-octave fbm — small enough to inline.
    "float h(vec2 p){return fract(sin(dot(p,vec2(127.1,311.7)))*43758.5453);}",
    "float n(vec2 p){vec2 i=floor(p);vec2 f=fract(p);vec2 u=f*f*(3.0-2.0*f);",
    "  return mix(mix(h(i),h(i+vec2(1.0,0.0)),u.x),mix(h(i+vec2(0.0,1.0)),h(i+vec2(1.0,1.0)),u.x),u.y);}",
    "float fbm(vec2 p){float v=0.0;float a=0.5;for(int i=0;i<4;i++){v+=a*n(p);p*=2.02;a*=0.5;}return v;}",
    "void main(){",
    "  vec2 uv=gl_FragCoord.xy/u_res;",
    "  vec2 p=vec2(uv.x*(u_res.x/u_res.y),uv.y);",
    // Sky gradient (top → bottom).
    "  vec3 col=mix(u_skyBot,u_skyTop,smoothstep(0.0,1.0,uv.y));",
    // Sun in upper-left of canvas, slow horizontal drift.
    "  vec2 sunPos=vec2(0.18+0.04*sin(u_time*0.1),0.78);",
    "  float sunDist=length((p-vec2(sunPos.x*(u_res.x/u_res.y),sunPos.y))*vec2(1.0,1.0));",
    "  float sunCore=smoothstep(0.10,0.0,sunDist);",
    "  float sunGlow=smoothstep(0.45,0.0,sunDist)*0.5;",
    "  col+=u_sun*(sunCore*vec3(1.0,0.95,0.78)+sunGlow*vec3(1.0,0.85,0.55));",
    // Clouds — two layers of fbm scrolling at different speeds.
    "  float cl1=fbm(p*2.5+vec2(u_time*0.04,0.0));",
    "  float cl2=fbm(p*5.0+vec2(u_time*0.08,2.5));",
    "  float clouds=smoothstep(1.0-u_cloud*0.7,1.0,cl1*0.7+cl2*0.4);",
    "  col=mix(col,vec3(0.96,0.96,0.98),clouds*0.85);",
    // Rain — diagonal streaks via masked sin.
    "  if(u_kind==2){",
    "    vec2 rp=p*vec2(120.0,40.0)+vec2(u_time*-2.0,u_time*16.0);",
    "    float streak=step(0.92,fract(rp.x)*step(0.6,fract(rp.y*0.5+h(floor(rp.xx)))));",
    "    col=mix(col,vec3(0.78,0.86,0.95),streak*0.55);",
    "  }",
    // Snow — drifting flakes via noise particles.
    "  if(u_kind==3){",
    "    for(int i=0;i<3;i++){",
    "      float fi=float(i);",
    "      vec2 sp=p*vec2(20.0+fi*8.0,18.0)+vec2(u_time*(0.3+fi*0.1),u_time*(1.4+fi*0.3));",
    "      vec2 ip=floor(sp);vec2 fp=fract(sp);",
    "      float r=h(ip);",
    "      float d=length(fp-vec2(0.5+0.3*sin(r*6.28+u_time),0.5));",
    "      col=mix(col,vec3(1.0),smoothstep(0.05,0.0,d)*step(0.85,r)*0.9);",
    "    }",
    "  }",
    // Storm — same rain effect + intermittent lightning flash.
    "  if(u_kind==4){",
    "    vec2 rp=p*vec2(140.0,50.0)+vec2(u_time*-3.0,u_time*22.0);",
    "    float streak=step(0.93,fract(rp.x)*step(0.5,fract(rp.y*0.5+h(floor(rp.xx)))));",
    "    col=mix(col,vec3(0.80,0.85,0.95),streak*0.6);",
    "    float flash=step(0.985,fract(sin(floor(u_time*1.2)*43.0)*4537.0));",
    "    col+=flash*vec3(1.0,0.95,0.85)*0.6;",
    "  }",
    // Fog wash — flatten contrast.
    "  if(u_kind==5){col=mix(col,vec3(0.86,0.87,0.89),0.55);}",
    // Soft vignette so the card edge feels intentional, not a hard cut.
    "  float vig=smoothstep(1.4,0.5,length((uv-0.5)*vec2(1.0,1.6)));",
    "  col*=mix(0.85,1.0,vig);",
    "  gl_FragColor=vec4(col,1.0);",
    "}",
  ].join("\n");

  function compile(gl, type, src) {
    var s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
      console.error("shader compile failed", gl.getShaderInfoLog(s));
      return null;
    }
    return s;
  }

  var glState = null;

  function initGL(canvas) {
    var gl = canvas.getContext("webgl") || canvas.getContext("experimental-webgl");
    if (!gl) return null;
    var vs = compile(gl, gl.VERTEX_SHADER, VS);
    var fs = compile(gl, gl.FRAGMENT_SHADER, FS);
    if (!vs || !fs) return null;
    var prog = gl.createProgram();
    gl.attachShader(prog, vs); gl.attachShader(prog, fs);
    gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
      console.error("link failed", gl.getProgramInfoLog(prog));
      return null;
    }
    gl.useProgram(prog);
    var buf = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1, 3,-1, -1,3]), gl.STATIC_DRAW);
    var loc = gl.getAttribLocation(prog, "a");
    gl.enableVertexAttribArray(loc);
    gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0);
    return {
      gl: gl, prog: prog,
      uRes:    gl.getUniformLocation(prog, "u_res"),
      uTime:   gl.getUniformLocation(prog, "u_time"),
      uSkyTop: gl.getUniformLocation(prog, "u_skyTop"),
      uSkyBot: gl.getUniformLocation(prog, "u_skyBot"),
      uCloud:  gl.getUniformLocation(prog, "u_cloud"),
      uSun:    gl.getUniformLocation(prog, "u_sun"),
      uKind:   gl.getUniformLocation(prog, "u_kind"),
      profile: profileFor(0),
      start:   performance.now(),
      canvas:  canvas,
    };
  }

  function resize(s) {
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    var w = Math.floor(s.canvas.clientWidth * dpr);
    var h = Math.floor(s.canvas.clientHeight * dpr);
    if (s.canvas.width !== w || s.canvas.height !== h) {
      s.canvas.width = w; s.canvas.height = h;
      s.gl.viewport(0, 0, w, h);
    }
  }

  function frame() {
    if (!glState) return;
    var s = glState;
    resize(s);
    var t = (performance.now() - s.start) / 1000;
    s.gl.uniform2f(s.uRes, s.canvas.width, s.canvas.height);
    s.gl.uniform1f(s.uTime, t);
    s.gl.uniform3fv(s.uSkyTop, s.profile.topA);
    s.gl.uniform3fv(s.uSkyBot, s.profile.topB);
    s.gl.uniform1f(s.uCloud, s.profile.cloud);
    s.gl.uniform1f(s.uSun, s.profile.sun);
    s.gl.uniform1i(s.uKind, s.profile.kind);
    s.gl.drawArrays(s.gl.TRIANGLES, 0, 3);
    requestAnimationFrame(frame);
  }

  function escapeHtml(x) { return String(x).replace(/[&<>"']/g, function (c) { return ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[c]; }); }

  function notifyHeight() {
    var h = Math.max(document.body.scrollHeight, 84);
    if (window.evey && window.evey.resize) window.evey.resize(h);
  }

  function mount() {
    document.body.style.margin = "0";
    document.body.style.background = "transparent";
    document.body.innerHTML = ''
      + '<div id="wrap" style="position:relative;width:240px;height:84px;margin-left:auto;border-radius:14px;overflow:hidden;font-family:system-ui,-apple-system,sans-serif;color:#fff;text-shadow:0 1px 2px rgba(0,0,0,0.45);box-shadow:0 6px 18px rgba(0,0,0,0.18)">'
      + '  <canvas id="cv" style="position:absolute;inset:0;width:100%;height:100%;display:block"></canvas>'
      + '  <div id="ov" style="position:absolute;inset:0;display:flex;flex-direction:column;justify-content:center;align-items:flex-end;padding:10px 14px;pointer-events:none">'
      + '    <div id="temp" style="font-size:30px;font-weight:600;line-height:1">…</div>'
      + '    <div id="meta" style="font-size:11px;opacity:0.9;text-transform:uppercase;letter-spacing:0.06em;margin-top:4px">Loading</div>'
      + '  </div>'
      + '</div>';
    var canvas = document.getElementById("cv");
    glState = initGL(canvas);
    if (!glState) {
      // Fallback: solid gradient if WebGL unavailable.
      document.getElementById("wrap").style.background = "linear-gradient(135deg,#1e3a8a,#3b82f6)";
    } else {
      requestAnimationFrame(frame);
    }
    notifyHeight();
  }

  function setWeather(state) {
    if (!glState) return;
    var p = profileFor(state.code);
    glState.profile = p;
    document.getElementById("temp").textContent = Math.round(state.temp) + (state.units === "fahrenheit" ? "°F" : "°C");
    document.getElementById("meta").textContent = state.city + " · " + p.label;
  }

  function setError(msg) {
    var meta = document.getElementById("meta");
    var temp = document.getElementById("temp");
    if (meta) meta.textContent = "Unavailable";
    if (temp) temp.textContent = "—";
    if (msg) console.warn("[weather]", msg);
  }

  async function geocode(city) {
    var r = await fetch("https://geocoding-api.open-meteo.com/v1/search?count=1&language=en&name=" + encodeURIComponent(city));
    if (!r.ok) throw new Error("geocoding HTTP " + r.status);
    var d = await r.json();
    if (!d.results || d.results.length === 0) throw new Error("city not found: " + city);
    return { lat: d.results[0].latitude, lon: d.results[0].longitude, name: d.results[0].name };
  }

  async function fetchWeather(lat, lon, units) {
    var url = "https://api.open-meteo.com/v1/forecast?current_weather=true&latitude=" + lat + "&longitude=" + lon + (units === "fahrenheit" ? "&temperature_unit=fahrenheit" : "");
    var r = await fetch(url);
    if (!r.ok) throw new Error("forecast HTTP " + r.status);
    var d = await r.json();
    if (!d.current_weather) throw new Error("no current_weather");
    return { temp: d.current_weather.temperature, code: d.current_weather.weathercode };
  }

  async function load(config) {
    var city = (config && config.city) || "Paris";
    var units = (config && config.units) || "celsius";
    try {
      var geo = await geocode(city);
      var w = await fetchWeather(geo.lat, geo.lon, units);
      setWeather({ city: geo.name, temp: w.temp, code: w.code, units: units });
    } catch (e) {
      setError(e && e.message);
    }
  }

  function start(ctx) {
    mount();
    load((ctx && ctx.config) || {});
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", function () { window.evey.onInit(start); });
  } else {
    window.evey.onInit(start);
  }
})();
