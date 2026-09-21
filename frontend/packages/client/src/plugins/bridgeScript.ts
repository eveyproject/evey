// The guest side of the plugin protocol: the script injected ahead of a
// plugin's bundle, on every surface. It exposes `window.evey` and carries the
// "ready" / "init" lifecycle and the RPC promises. One script, two
// transports, picked at run time:
//   - a React Native WebView delivers guest -> host through
//     window.ReactNativeWebView.postMessage (strings only) and host -> guest
//     through injected `window.postMessage` calls;
//   - the web's sandboxed iframe uses window.parent.postMessage both ways.
// So the same plugin code runs unchanged on web, mobile and TV.
export interface EveyBridge {
  navigate(path: string): void;
  toast(level: "success" | "error" | "info", message: string): void;
  resize(height: number): void;
  onInit(callback: (data: { context: Record<string, unknown>; locale: string; theme: string }) => void): void;
  // Server-backed APIs, gated by the granted plugin permissions (`prefs` /
  // `public`). The host proxies these with a scoped plugin token, so a plugin
  // only reaches what it was granted; a missing scope rejects the promise.
  prefs: {
    get(key: string): Promise<{ value: string; found: boolean }>;
    set(key: string, value: string): Promise<void>;
    delete(key: string): Promise<void>;
    list(): Promise<{ key: string; value: string }[]>;
  };
  media: {
    get(id: string): Promise<unknown>;
    list(opts?: { libraryId?: string }): Promise<unknown>;
    // The media's poster as a data: URL, usable directly as a WebGL/img
    // source - the sandbox CSP forbids external CDN images and its opaque
    // origin taints cross-origin textures, so a plugin cannot load library
    // posters any other way.
    poster(id: string): Promise<string>;
  };
  // Reads one of the plugin's OWN web_root assets (by relative path) and
  // returns it as a data: URL. The host fetches it with the scoped plugin
  // token, so a runtime-written asset (e.g. an image the plugin backend just
  // downloaded) becomes usable as a WebGL texture inside the sandbox without
  // any CORS/taint concern. Scoped to the plugin's own assets only - not an
  // arbitrary-URL fetch.
  assets: {
    read(path: string): Promise<string>;
  };
}

export const BRIDGE_SCRIPT = `
(function() {
  var rn = (window.ReactNativeWebView && window.ReactNativeWebView.postMessage) ? window.ReactNativeWebView : null;
  var initCallbacks = [];
  var initData = null; // captured init payload, replayed to late onInit subscribers
  var rpcSeq = 0;
  var rpcPending = {};
  function send(msg) {
    if (rn) rn.postMessage(JSON.stringify(msg));
    else window.parent.postMessage(msg, "*");
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
      poster: function(id) { return rpc("media.poster", { id: id }); }
    },
    assets: {
      read: function(path) { return rpc("assets.read", { path: path }); }
    },
    onInit: function(cb) {
      initCallbacks.push(cb);
      // Replay if init has already been received (the bundle loads after the
      // bridge runs and posts "ready", so the host's init reply may have
      // already arrived by the time the module registers its handler).
      if (initData) {
        try { cb(initData); } catch(err) { console.error("[evey-front]", err); }
      }
    }
  };
  window.addEventListener("message", function(e) {
    // In the iframe, only the direct embedder may speak: the sandbox (no
    // allow-same-origin) gives an opaque "null" origin, so e.origin is not a
    // reliable check and e.source === window.parent is the guard. A WebView
    // has no parent; the host injects its messages as window.postMessage.
    if (!rn && e.source !== window.parent) return;
    var data = e.data;
    if (!data) return;
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
  // Defer the "ready" signal until after the body's <script> tags have
  // executed, so onInit callbacks they register don't miss the init reply.
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
  if (rn) {
    var nudgeResize = function() {
      try { window.dispatchEvent(new Event("resize")); } catch (_) {}
    };
    setTimeout(nudgeResize, 60);
    setTimeout(nudgeResize, 240);
  }
})();
`;
