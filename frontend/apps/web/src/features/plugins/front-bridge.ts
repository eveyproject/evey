export interface EveyBridge {
  navigate(path: string): void;
  toast(level: "success" | "error" | "info", message: string): void;
  resize(height: number): void;
  onInit(callback: (data: { context: Record<string, unknown>; locale: string; theme: string }) => void): void;
}

export const BRIDGE_SCRIPT = `
(function() {
  var initCallbacks = [];
  var initData = null; // captured init payload, replayed to late onInit subscribers
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
    if (e.data && e.data.type === "init") {
      initData = e.data;
      for (var i = 0; i < initCallbacks.length; i++) {
        try { initCallbacks[i](e.data); } catch(err) { console.error("[evey-front]", err); }
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
