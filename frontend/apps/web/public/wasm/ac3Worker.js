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

/* wasmMemory is the module's linear memory. It is the ONLY honest memory
 * figure for this decoder: performance.memory (the receiver's mem= beacon)
 * reports the JS heap, which a growing WebAssembly.Memory never appears in.
 * A cast that dies with a flat heap beacon was therefore unexplained - see
 * the 2026-07-30 Bouygtel4K incident. */
let wasmMemory = null;
function wasmMemMB() {
  try {
    return wasmMemory ? Math.round(wasmMemory.buffer.byteLength / 1048576) : -1;
  } catch (_) {
    return -1;
  }
}

/* loadRuntime imports the shim + instantiates the wasm module, once.
 * bustCache=true bypasses the HTTP cache on BOTH files: the shim and the wasm
 * are only compatible within the same build generation (standard-go and tinygo
 * wasm_exec.js are mutually incompatible), and without cache-busting a redeploy
 * could pair a cached old half with a fresh new one - an instant LinkError
 * (seen in prod 2026-07-26 on the go->tinygo switch). importScripts has no
 * cache option, so the bust rides a throwaway query param instead. */
async function loadRuntime(wasmUrl, wasmExecUrl, bustCache) {
  // wasm_exec.js is Go's runtime shim; it defines self.Go. Classic worker →
  // importScripts pulls it in synchronously without eval. Re-importing on the
  // retry simply reassigns self.Go - both shim flavors assign the global.
  const execUrl = bustCache
    ? wasmExecUrl + (wasmExecUrl.includes("?") ? "&" : "?") + "reload=" + Date.now()
    : wasmExecUrl;
  importScripts(execUrl);
  const go = new self.Go();
  let instance;
  if (bustCache) {
    const resp = await fetch(wasmUrl, { cache: "reload" });
    if (!resp.ok) throw new Error("fetch " + wasmUrl + " -> " + resp.status);
    const res = await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
    wasmMemory = res.instance.exports.mem || null;
    instance = res.instance;
  } else {
    // instantiateStreaming first: it's the ONLY form Chrome's compiled-code
    // cache (keyed by URL) can reuse - decisive on a TV CPU (cast receiver)
    // where compilation dominates the time-to-sound, and it lets the page's
    // compileStreaming warmup pay off ahead of time.
    // Bytes fallback for engines without streaming or a missing Content-Type.
    try {
      const res = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
      wasmMemory = res.instance.exports.mem || null;
      instance = res.instance;
    } catch (_) {
      const resp = await fetch(wasmUrl);
      if (!resp.ok) throw new Error("fetch " + wasmUrl + " -> " + resp.status);
      const bytes = await resp.arrayBuffer();
      const res = await WebAssembly.instantiate(bytes, go.importObject);
      wasmMemory = res.instance.exports.mem || null;
      instance = res.instance;
    }
  }
  // go.run keeps the Go runtime alive (the module's main is `select{}`); do not
  // await it - it never resolves. The exported callbacks are ready synchronously
  // once run has registered the global.
  go.run(instance);
  const mod = self.Ac3Go;
  if (!mod || typeof mod.decode !== "function") {
    throw new Error("ac3go module did not register a decode()");
  }
  return mod;
}

async function init(wasmUrl, wasmExecUrl) {
  try {
    ac3 = await loadRuntime(wasmUrl, wasmExecUrl, false);
  } catch (firstErr) {
    // Self-healing: a first failure is most often the stale-cache mix above -
    // retry ONCE with the HTTP cache bypassed before giving up (giving up
    // latches the host's fallback to the AAC transcode for the whole title).
    try {
      ac3 = await loadRuntime(wasmUrl, wasmExecUrl, true);
    } catch (retryErr) {
      throw new Error(
        "init failed (" + String((firstErr && firstErr.message) || firstErr) +
        "); cache-bypass retry failed (" + String((retryErr && retryErr.message) || retryErr) + ")",
      );
    }
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
          wasmMemMB: wasmMemMB(),
        },
        [pcm],
      );
    } catch (err) {
      post({ type: "decodeError", id, error: String((err && err.message) || err) });
    }
  }
};
