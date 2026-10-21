// Feature probe, not a user-agent table: a television browser that
// lacks any of these cannot run the app, whatever its brand. Runs
// before the module script so an ancient engine that cannot even
// parse the bundle still shows a readable screen. Kept in ES5 for the
// same reason. A file of its own, not an inline script: the app's CSP has no
// 'unsafe-inline' in script-src, so inline it never ran under /tv.
(function () {
  var ok = true;
  try {
    ok = typeof Promise !== "undefined" && typeof fetch !== "undefined" &&
      typeof Symbol !== "undefined" && typeof Map !== "undefined" &&
      !!(window.MediaSource || window.ManagedMediaSource) &&
      "noModule" in document.createElement("script");
  // ES5 has no catch without a binding (ES2019), and this file must parse there.
  // eslint-disable-next-line no-unused-vars
  } catch (e) { ok = false; }
  if (!ok) {
    window.__eveyTooOld = true;
    document.addEventListener("DOMContentLoaded", function () {
      var el = document.getElementById("too-old");
      if (el) el.style.display = "block";
    });
  }
})();
