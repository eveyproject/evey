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
  };
  // Reads one of the plugin's OWN web_root assets (by relative path) and
  // returns it as a data: URL. The host fetches it same-origin with the
  // scoped plugin token, so a runtime-written asset (e.g. an image the plugin
  // backend just downloaded) becomes usable as a WebGL texture inside the
  // sandboxed, opaque-origin iframe without any CORS/taint concern. Scoped to
  // the plugin's own assets only - not an arbitrary-URL fetch.
  assets: {
    read(path: string): Promise<string>;
  };
}

export const BRIDGE_SCRIPT = `
(function() {
  var initCallbacks = [];
  var initData = null; // captured init payload, replayed to late onInit subscribers
  var rpcSeq = 0;
  var rpcPending = {};
  function rpc(method, args) {
    return new Promise(function(resolve, reject) {
      var id = ++rpcSeq;
      rpcPending[id] = { resolve: resolve, reject: reject };
      window.parent.postMessage({ type: "rpc", id: id, method: method, args: args || {} }, "*");
    });
  }
  window.evey = {
    navigate: function(path) {
      window.parent.postMessage({ type: "navigate", path: path }, "*");
    },
    toast: function(level, message) {
      window.parent.postMessage({ type: "toast", level: level, message: message }, "*");
    },
    resize: function(height) {
      window.parent.postMessage({ type: "resize", height: height }, "*");
    },
    prefs: {
      get: function(key) { return rpc("prefs.get", { key: key }); },
      set: function(key, value) { return rpc("prefs.set", { key: key, value: value }); },
      delete: function(key) { return rpc("prefs.delete", { key: key }); },
      list: function() { return rpc("prefs.list", {}); }
    },
    media: {
      get: function(id) { return rpc("media.get", { id: id }); },
      list: function(opts) { return rpc("media.list", opts || {}); }
    },
    assets: {
      read: function(path) { return rpc("assets.read", { path: path }); }
    },
    onInit: function(cb) {
      initCallbacks.push(cb);
      // Replay if init has already been received (front.js loads after the
      // bridge runs and posts "ready", so the parent's init reply may have
      // already arrived by the time the front module registers its handler).
      if (initData) {
        try { cb(initData); } catch(err) { console.error("[evey-front]", err); }
      }
    }
  };
  window.addEventListener("message", function(e) {
    // Only accept messages originating from the parent frame. The iframe runs
    // with sandbox (no allow-same-origin), so its origin is opaque "null" —
    // checking e.origin is not reliable here. e.source === window.parent is
    // the authoritative guard: only the direct embedder can send the init payload.
    if (e.source !== window.parent) return;
    if (e.data && e.data.type === "init") {
      initData = e.data;
      for (var i = 0; i < initCallbacks.length; i++) {
        try { initCallbacks[i](e.data); } catch(err) { console.error("[evey-front]", err); }
      }
    } else if (e.data && e.data.type === "rpc-result") {
      var p = rpcPending[e.data.id];
      if (p) {
        delete rpcPending[e.data.id];
        if (e.data.ok) p.resolve(e.data.data);
        else p.reject(new Error(e.data.error || "rpc error"));
      }
    }
  });
  // Defer the "ready" signal until after the body's <script> tags have
  // executed, so onInit callbacks they register don't miss the init reply.
  if (document.readyState === "complete" || document.readyState === "interactive") {
    queueMicrotask(function() { window.parent.postMessage({ type: "ready" }, "*"); });
  } else {
    window.addEventListener("DOMContentLoaded", function() {
      window.parent.postMessage({ type: "ready" }, "*");
    });
  }
})();
`;
