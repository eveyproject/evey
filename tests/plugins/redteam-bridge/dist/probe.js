// Runs inside the sandboxed iframe. Attempts to bypass each new security layer
// and renders the verdict per probe so the host (and a Playwright test) can read
// it. window.evey is the ONLY sanctioned channel; everything else should fail.
(function () {
  var lines = [];
  function render() {
    document.body.innerHTML =
      "<div data-testid='rt-results' style='font:12px monospace;padding:8px'>" +
      lines.map(function (l) { return l.replace(/</g, "&lt;"); }).join("<br>") +
      "</div>";
    try { window.evey.resize(Math.min(400, 30 + lines.length * 18)); } catch (e) {}
  }
  function add(name, verdict, detail) { lines.push(name + "=" + verdict + (detail ? " (" + detail + ")" : "")); render(); }

  // 1+2: direct calls to the evey API from the iframe must be blocked by CSP
  // (connect-src is the plugin's external_fetch list - empty here).
  fetch("/api/v1/admin/plugin-resource-limits")
    .then(function (r) { add("direct_admin_fetch", "BYPASSED", "HTTP " + r.status); })
    .catch(function (e) { add("direct_admin_fetch", "BLOCKED", String(e.name || e).slice(0, 30)); });
  fetch("/api/v1/plugin-prefs")
    .then(function (r) { add("direct_prefs_fetch", "BYPASSED", "HTTP " + r.status); })
    .catch(function (e) { add("direct_prefs_fetch", "BLOCKED", String(e.name || e).slice(0, 30)); });

  // 3: the sanctioned, granted path - prefs round-trip in OUR own namespace.
  window.evey.prefs.set("rt", "42")
    .then(function () { return window.evey.prefs.get("rt"); })
    .then(function (g) { add("bridge_prefs_roundtrip", g && g.found && g.value === "42" ? "OK" : "FAIL", "v=" + (g && g.value)); })
    .catch(function (e) { add("bridge_prefs_roundtrip", "ERROR", String(e.message || e).slice(0, 30)); });

  // 4: the public media projection must never leak a file path.
  window.evey.media.list({})
    .then(function (m) {
      var items = (m && m.items) || [];
      var leak = items.some(function (it) { return it && (it.filePath || it.file_path); });
      add("media_filepath_leak", leak ? "BYPASSED" : "BLOCKED", items.length + " items");
    })
    .catch(function (e) { add("media_list", "ERROR", String(e.message || e).slice(0, 30)); });

  render();
})();
