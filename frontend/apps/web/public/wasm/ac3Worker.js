/* ac3Worker.js - a classic Web Worker that runs the ac3go WebAssembly decoder
 * off the main thread. It loads Go's wasm runtime shim + the ac3go module once,
 * then decodes AC-3 / E-AC-3 CMAF audio segments to interleaved float32 PCM on
 * demand. Kept as a plain-JS file in public/ (not bundled) so it can be a
 * CLASSIC worker and use importScripts for the non-module wasm_exec.js - the one
 * reliable way to pull in Go's runtime without eval (CSP-safe beyond the
 * wasm-unsafe-eval WebAssembly itself needs).
 *
 * Protocol (main <-> worker), all postMessage:
 *   main -> { type:'init', wasmUrl, wasmExecUrl }
 *   worker -> { type:'ready' } | { type:'error', error }
 *   main -> { type:'decode', id, bytes:ArrayBuffer, downmix? } (bytes transferred)
 *   worker -> { type:'decoded', id, channels, sampleRate, frames, pcm:ArrayBuffer }
 *           | { type:'decodeError', id, error }
 */

let ac3 = null;

function post(msg, transfer) {
  self.postMessage(msg, transfer || []);
}

async function init(wasmUrl, wasmExecUrl) {
  // wasm_exec.js is Go's runtime shim; it defines self.Go. Classic worker →
  // importScripts pulls it in synchronously without eval.
  importScripts(wasmExecUrl);
  const go = new self.Go();
  // instantiateStreaming d'abord : c'est la SEULE forme que le cache de code
  // compilé de Chrome (clé = URL) sait réutiliser - déterminant sur un CPU de
  // TV (receiver cast) où la compilation domine le délai d'arrivée du son, et
  // ça permet au préchauffage compileStreaming de la page de payer d'avance.
  // Repli bytes pour les moteurs sans streaming ou un Content-Type manquant.
  let instance;
  try {
    const res = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
    instance = res.instance;
  } catch (_) {
    const resp = await fetch(wasmUrl);
    if (!resp.ok) throw new Error("fetch " + wasmUrl + " -> " + resp.status);
    const bytes = await resp.arrayBuffer();
    const res = await WebAssembly.instantiate(bytes, go.importObject);
    instance = res.instance;
  }
  // go.run keeps the Go runtime alive (the module's main is `select{}`); do not
  // await it - it never resolves. The exported callbacks are ready synchronously
  // once run has registered the global.
  go.run(instance);
  ac3 = self.Ac3Go;
  if (!ac3 || typeof ac3.decode !== "function") {
    throw new Error("ac3go module did not register a decode()");
  }
}

self.onmessage = async (e) => {
  const msg = e.data;
  if (!msg) return;

  if (msg.type === "init") {
    try {
      await init(msg.wasmUrl, msg.wasmExecUrl);
      post({ type: "ready" });
    } catch (err) {
      post({ type: "error", error: String((err && err.message) || err) });
    }
    return;
  }

  if (msg.type === "decode") {
    const { id } = msg;
    if (!ac3) {
      post({ type: "decodeError", id, error: "decoder not initialised" });
      return;
    }
    try {
      const opts = msg.downmix ? { downmix: msg.downmix } : undefined;
      const res = ac3.decode(new Uint8Array(msg.bytes), opts);
      if (res.error) {
        post({ type: "decodeError", id, error: res.error });
        return;
      }
      const floatCount = res.frames * res.channels;
      // Copy the exact PCM span into a fresh ArrayBuffer so it transfers cleanly
      // (the Go-returned view may alias runtime memory we must not detach).
      const view = new Uint8Array(res.bytes.buffer, res.bytes.byteOffset, floatCount * 4);
      const pcm = view.slice().buffer;
      post(
        {
          type: "decoded",
          id,
          channels: res.channels,
          sampleRate: res.sampleRate,
          frames: res.frames,
          pcm,
        },
        [pcm],
      );
    } catch (err) {
      post({ type: "decodeError", id, error: String((err && err.message) || err) });
    }
  }
};
