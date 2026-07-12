// Config Demo Front - displays the plugin's current configuration
(function () {
  var root = document.body;

  window.evey.onInit(function (ctx) {
    var config = (ctx && ctx.config) || {};
    var keys = Object.keys(config);

    var html = '<div style="padding:16px;font-family:system-ui,sans-serif">';
    html += "<h2>Plugin Configuration</h2>";

    if (keys.length === 0) {
      html += "<p>No configuration set yet.</p>";
    } else {
      html += "<table style='border-collapse:collapse;width:100%'>";
      html += "<tr><th style='text-align:left;padding:4px 8px;border-bottom:1px solid #ccc'>Key</th>";
      html += "<th style='text-align:left;padding:4px 8px;border-bottom:1px solid #ccc'>Value</th></tr>";
      keys.forEach(function (k) {
        var val = k === "api_key" ? "********" : String(config[k]);
        html += "<tr><td style='padding:4px 8px'>" + k + "</td>";
        html += "<td style='padding:4px 8px'>" + val + "</td></tr>";
      });
      html += "</table>";
    }

    html += "</div>";
    root.innerHTML = html;
  });
})();
