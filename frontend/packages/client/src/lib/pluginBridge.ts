// Bridge script injected into the WebView that hosts a plugin's frontend
// bundle. Mirrors the web `front-bridge.ts` 1:1 so the same plugin code
// runs unchanged on both platforms - postMessage protocol, "ready" /
// "init" lifecycle, evey.{navigate,toast,resize,prefs,media,assets} surface.
//
// React-Native WebView delivers messages via window.ReactNativeWebView
// (host → guest direction is window.postMessage of a serialized event).
// For the GUEST → HOST direction we use window.ReactNativeWebView.postMessage
// (string only). For HOST → GUEST we inject `window.postMessage` calls via
// the WebView's injectJavaScript bridge.
export const PLUGIN_BRIDGE_SCRIPT = `
(function() {
  var initCallbacks = [];
  var initData = null;
  var rpcSeq = 0;
  var rpcPending = {};
  function send(msg) {
    if (window.ReactNativeWebView && window.ReactNativeWebView.postMessage) {
      window.ReactNativeWebView.postMessage(JSON.stringify(msg));
    }
  }
  function rpc(method, args) {
    return new Promise(function(resolve, reject) {
      var id = ++rpcSeq;
      rpcPending[id] = { resolve: resolve, reject: reject };
      send({ type: "rpc", id: id, method: method, args: args || {} });
    });
  }
  window.evey = {
    navigate: function(path) { send({ type: "navigate", path: path }); },
    toast: function(level, message) { send({ type: "toast", level: level, message: message }); },
    resize: function(height) { send({ type: "resize", height: height }); },
    prefs: {
      get: function(key) { return rpc("prefs.get", { key: key }); },
      set: function(key, value) { return rpc("prefs.set", { key: key, value: value }); },
      delete: function(key) { return rpc("prefs.delete", { key: key }); },
      list: function() { return rpc("prefs.list", {}); }
    },
    media: {
      get: function(id) { return rpc("media.get", { id: id }); },
      list: function(opts) { return rpc("media.list", opts || {}); },
      // Returns the media's poster as a data: URL. Mirrors web's evey.media.poster
      // (no in-host downscale here - RN has no canvas, see PluginSlot.tsx).
      poster: function(id) { return rpc("media.poster", { id: id }); }
    },
    assets: {
      read: function(path) { return rpc("assets.read", { path: path }); }
    },
    onInit: function(cb) {
      initCallbacks.push(cb);
      if (initData) {
        try { cb(initData); } catch(err) { console.error("[evey-front]", err); }
      }
    }
  };
  window.addEventListener("message", function(e) {
    if (!e.data) return;
    var data = e.data;
    if (typeof data === "string") {
      try { data = JSON.parse(data); } catch(_) { return; }
    }
    if (data && data.type === "init") {
      initData = data;
      for (var i = 0; i < initCallbacks.length; i++) {
        try { initCallbacks[i](data); } catch(err) { console.error("[evey-front]", err); }
      }
    } else if (data && data.type === "rpc-result") {
      var p = rpcPending[data.id];
      if (p) {
        delete rpcPending[data.id];
        if (data.ok) p.resolve(data.data);
        else p.reject(new Error(data.error || "rpc error"));
      }
    }
  });
  if (document.readyState === "complete" || document.readyState === "interactive") {
    queueMicrotask(function() { send({ type: "ready" }); });
  } else {
    window.addEventListener("DOMContentLoaded", function() { send({ type: "ready" }); });
  }
  // Android WebView often computes a 0-width viewport on first layout when
  // the bundle inlines a heavy framework - three.js / react-three-fiber
  // measure their parent on mount and never re-measure on a non-bounding-
  // box change. Dispatch a "resize" once after the plugin had a tick to
  // mount so R3F recomputes against the now-real WebView width.
  function nudgeResize() {
    try { window.dispatchEvent(new Event("resize")); } catch (_) {}
  }
  setTimeout(nudgeResize, 60);
  setTimeout(nudgeResize, 240);
})();
`;
