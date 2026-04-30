// Weather front module — transparent, right-aligned card.
// CSP allows fetches to api.open-meteo.com + geocoding-api.open-meteo.com
// (declared in plugin.yaml external_fetch).

(function () {
  const ICONS = {
    sun: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><circle cx="32" cy="32" r="11" fill="#fbbf24"/><g stroke="#fbbf24" stroke-width="3" stroke-linecap="round"><line x1="32" y1="6" x2="32" y2="14"/><line x1="32" y1="50" x2="32" y2="58"/><line x1="6" y1="32" x2="14" y2="32"/><line x1="50" y1="32" x2="58" y2="32"/><line x1="13" y1="13" x2="19" y2="19"/><line x1="45" y1="45" x2="51" y2="51"/><line x1="51" y1="13" x2="45" y2="19"/><line x1="19" y1="45" x2="13" y2="51"/></g></svg>',
    cloud: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><path d="M18 42c-6 0-10-4-10-9 0-4 3-8 8-9 1-7 7-12 14-12 8 0 14 6 14 14h2c5 0 9 4 9 9s-4 9-9 9H18z" fill="#cbd5e1" stroke="#94a3b8" stroke-width="2"/></svg>',
    partly: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><circle cx="22" cy="22" r="9" fill="#fbbf24"/><g stroke="#fbbf24" stroke-width="2.5" stroke-linecap="round"><line x1="22" y1="4" x2="22" y2="9"/><line x1="4" y1="22" x2="9" y2="22"/><line x1="9" y1="9" x2="13" y2="13"/><line x1="35" y1="9" x2="31" y2="13"/></g><path d="M22 46c-5 0-9-3-9-8 0-3 2-6 6-7 1-5 5-9 11-9 6 0 11 4 11 10h2c4 0 7 3 7 7s-3 7-7 7H22z" fill="#e2e8f0" stroke="#94a3b8" stroke-width="2"/></svg>',
    rain: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><path d="M18 36c-6 0-10-4-10-9 0-4 3-8 8-9 1-7 7-12 14-12 8 0 14 6 14 14h2c5 0 9 4 9 9s-4 9-9 9H18z" fill="#94a3b8" stroke="#64748b" stroke-width="2"/><g stroke="#3b82f6" stroke-width="3" stroke-linecap="round"><line x1="22" y1="46" x2="19" y2="56"/><line x1="32" y1="46" x2="29" y2="56"/><line x1="42" y1="46" x2="39" y2="56"/></g></svg>',
    snow: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><path d="M18 36c-6 0-10-4-10-9 0-4 3-8 8-9 1-7 7-12 14-12 8 0 14 6 14 14h2c5 0 9 4 9 9s-4 9-9 9H18z" fill="#e2e8f0" stroke="#94a3b8" stroke-width="2"/><g fill="#ffffff" stroke="#cbd5e1" stroke-width="1"><circle cx="22" cy="50" r="2.5"/><circle cx="32" cy="54" r="2.5"/><circle cx="42" cy="50" r="2.5"/></g></svg>',
    storm: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><path d="M18 36c-6 0-10-4-10-9 0-4 3-8 8-9 1-7 7-12 14-12 8 0 14 6 14 14h2c5 0 9 4 9 9s-4 9-9 9H18z" fill="#475569" stroke="#334155" stroke-width="2"/><polygon points="30,42 24,54 30,54 26,62 38,48 32,48 36,42" fill="#fbbf24" stroke="#d97706" stroke-width="1"/></svg>',
    fog: '<svg viewBox="0 0 64 64" width="44" height="44" fill="none"><g stroke="#94a3b8" stroke-width="3" stroke-linecap="round"><line x1="10" y1="22" x2="54" y2="22"/><line x1="6" y1="32" x2="58" y2="32" opacity="0.7"/><line x1="14" y1="42" x2="50" y2="42" opacity="0.5"/><line x1="10" y1="52" x2="54" y2="52" opacity="0.3"/></g></svg>',
  };

  function codeToIcon(code) {
    if (code === 0) return { icon: "sun", label: "Clear" };
    if (code === 1 || code === 2) return { icon: "partly", label: "Partly cloudy" };
    if (code === 3) return { icon: "cloud", label: "Overcast" };
    if (code >= 45 && code <= 48) return { icon: "fog", label: "Fog" };
    if (code >= 51 && code <= 67) return { icon: "rain", label: "Rain" };
    if (code >= 71 && code <= 77) return { icon: "snow", label: "Snow" };
    if (code >= 80 && code <= 82) return { icon: "rain", label: "Showers" };
    if (code >= 85 && code <= 86) return { icon: "snow", label: "Snow showers" };
    if (code >= 95) return { icon: "storm", label: "Thunderstorm" };
    return { icon: "cloud", label: "Unknown" };
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  function notifyHeight() {
    var h = Math.max(document.body.scrollHeight, 60);
    if (window.evey && window.evey.resize) window.evey.resize(h);
  }

  function render(state) {
    // Transparent body, content right-aligned. Color inherits from host theme
    // via the --text CSS variable forwarded by PluginIframe.
    var rowStyle = "display:flex;justify-content:flex-end;align-items:center;gap:10px;padding:6px 4px;font-family:system-ui,-apple-system,sans-serif;color:var(--text,#f1f5f9);";

    if (state.error) {
      document.body.innerHTML =
        '<div style="' + rowStyle + 'opacity:0.6;font-size:12px">Weather unavailable</div>';
      notifyHeight();
      return;
    }
    if (state.loading) {
      document.body.innerHTML =
        '<div style="' + rowStyle + 'opacity:0.5;font-size:12px">Loading weather…</div>';
      notifyHeight();
      return;
    }

    var ic = codeToIcon(state.code);
    var unit = state.units === "fahrenheit" ? "°F" : "°C";
    document.body.innerHTML =
      '<div style="' + rowStyle + '">' +
        '<div style="display:flex;flex-direction:column;align-items:flex-end;line-height:1.1">' +
          '<div style="font-size:22px;font-weight:600">' + Math.round(state.temp) + unit + '</div>' +
          '<div style="font-size:11px;opacity:0.7;text-transform:uppercase;letter-spacing:0.05em">' + escapeHtml(state.city) + '</div>' +
        '</div>' +
        '<div style="flex:0 0 auto;display:flex;align-items:center" title="' + escapeHtml(ic.label) + '">' + ICONS[ic.icon] + '</div>' +
      '</div>';
    notifyHeight();
  }

  async function geocode(city) {
    var url = "https://geocoding-api.open-meteo.com/v1/search?count=1&language=en&name=" + encodeURIComponent(city);
    var r = await fetch(url);
    if (!r.ok) throw new Error("geocoding HTTP " + r.status);
    var data = await r.json();
    if (!data.results || data.results.length === 0) throw new Error("city not found: " + city);
    return { lat: data.results[0].latitude, lon: data.results[0].longitude, name: data.results[0].name };
  }

  async function fetchWeather(lat, lon, units) {
    var unitParam = units === "fahrenheit" ? "&temperature_unit=fahrenheit" : "";
    var url = "https://api.open-meteo.com/v1/forecast?current_weather=true&latitude=" + lat + "&longitude=" + lon + unitParam;
    var r = await fetch(url);
    if (!r.ok) throw new Error("forecast HTTP " + r.status);
    var data = await r.json();
    if (!data.current_weather) throw new Error("no current_weather");
    return { temp: data.current_weather.temperature, code: data.current_weather.weathercode };
  }

  async function load(config) {
    var city = (config && config.city) || "Paris";
    var units = (config && config.units) || "celsius";
    render({ loading: true });
    try {
      var geo = await geocode(city);
      var w = await fetchWeather(geo.lat, geo.lon, units);
      render({ city: geo.name, temp: w.temp, code: w.code, units: units });
    } catch (e) {
      render({ error: e && e.message ? e.message : "fetch failed" });
    }
  }

  window.evey.onInit(function (ctx) {
    load((ctx && ctx.config) || {});
  });
})();
