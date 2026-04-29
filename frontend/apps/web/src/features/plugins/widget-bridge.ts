export interface EveyBridge {
  navigate(path: string): void;
  toast(level: "success" | "error" | "info", message: string): void;
  resize(height: number): void;
  onInit(callback: (data: { context: Record<string, unknown>; locale: string; theme: string }) => void): void;
}

export const BRIDGE_SCRIPT = `
(function() {
  var initCallbacks = [];
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
    }
  };
  window.addEventListener("message", function(e) {
    if (e.data && e.data.type === "init") {
      for (var i = 0; i < initCallbacks.length; i++) {
        try { initCallbacks[i](e.data); } catch(err) { console.error("[evey-front]", err); }
      }
    }
  });
  window.parent.postMessage({ type: "ready" }, "*");
})();
`;
