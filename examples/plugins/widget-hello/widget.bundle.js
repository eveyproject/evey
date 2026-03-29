// Hello Widget — runs inside a sandboxed iframe.
// Uses window.evey postMessage bridge injected by Evey host.
(function () {
  var root = document.getElementById("evey-widget") || document.body;

  // Wait for the Evey bridge to be ready
  window.evey.onInit(function (ctx) {
    var user = (ctx && ctx.user && ctx.user.name) || "User";

    root.innerHTML =
      '<div style="padding:16px;font-family:system-ui,sans-serif">' +
      "<h2>Hello, " + user + "!</h2>" +
      "<p>This widget is running inside a sandboxed iframe.</p>" +
      '<button id="greet">Show Toast</button>' +
      "</div>";

    document.getElementById("greet").addEventListener("click", function () {
      window.evey.toast({ message: "Greetings from hello-widget!", type: "info" });
    });
  });
})();
