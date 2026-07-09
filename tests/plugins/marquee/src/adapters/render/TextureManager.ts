// GL-side texture pool: wraps the pure LRU (domain/services/textureLru) with
// actual WebGLTexture objects and progressive same-origin image loading.
// Every poster gets a 1x1 placeholder texture immediately (so the wall never
// shows a hole), then its real photo streams in through a small bounded
// concurrency queue - not all ~30 requests fired at once. Capacity-bounded:
// once `capacity` textures are resident, reserving a new one evicts the
// least-recently-used (see domain/services/textureLru.ts for the tested
// eviction bookkeeping this delegates to).

import { createLru, type Lru } from "../../domain/services/textureLru";

interface TextureEntry {
  tex: WebGLTexture;
  loaded: boolean;
}

const MAX_CONCURRENT_LOADS = 4;


export class TextureManager {
  private lru: Lru<string>;
  private textures = new Map<string, TextureEntry>();
  private queue: Array<{ id: string; url: string }> = [];
  private activeLoads = 0;
  private disposed = false;

  constructor(private gl: WebGLRenderingContext, capacity: number) {
    this.lru = createLru<string>(capacity);
  }

  // Ensures a (placeholder, at least) texture exists for `id` and returns it.
  // Enqueues the real image load the first time this id is seen.
  reserve(id: string, url: string, placeholder: readonly [number, number, number]): WebGLTexture {
    const evicted = this.lru.touch(id);
    if (evicted !== null && evicted !== id) this.evict(evicted);

    let entry = this.textures.get(id);
    if (!entry) {
      entry = { tex: this.createPlaceholderTexture(placeholder), loaded: false };
      this.textures.set(id, entry);
      // Empty url (poster relay failed) -> keep the placeholder, never queue.
      if (url) this.queue.push({ id, url });
    }
    return entry.tex;
  }

  isLoaded(id: string): boolean {
    return this.textures.get(id)?.loaded ?? false;
  }

  // Starts up to MAX_CONCURRENT_LOADS pending image loads. Cheap no-op once
  // the queue has drained - safe to call every frame.
  pump(): void {
    while (this.activeLoads < MAX_CONCURRENT_LOADS && this.queue.length > 0) {
      const job = this.queue.shift();
      if (!job) break;
      const entry = this.textures.get(job.id);
      if (!entry) continue; // evicted before its turn came up

      this.activeLoads++;
      // The poster is a host-relayed data: URL (the sandbox CSP blocks the
      // external CDN URL). img-src allows data:, and a data URL is same-origin
      // in the iframe so the decoded <img> is not tainted for WebGL.
      const img = new Image();
      img.onload = () => {
        this.activeLoads--;
        if (this.disposed) return;
        const current = this.textures.get(job.id);
        if (!current) return; // evicted while loading
        const gl = this.gl;
        try {
          gl.bindTexture(gl.TEXTURE_2D, current.tex);
          gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, 0);
          gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGB, gl.RGB, gl.UNSIGNED_BYTE, img);
          gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, 0);
          current.loaded = true;
        } catch (e) {
          console.warn("marquee: poster texImage2D threw (tainted?)", e);
        }
      };
      img.onerror = () => {
        this.activeLoads--;
        // Left as the placeholder texture; never retried this session.
      };
      img.src = job.url;
    }
  }

  dispose(): void {
    this.disposed = true;
    for (const entry of this.textures.values()) this.gl.deleteTexture(entry.tex);
    this.textures.clear();
    this.queue.length = 0;
  }

  private evict(id: string): void {
    const entry = this.textures.get(id);
    if (!entry) return;
    this.gl.deleteTexture(entry.tex);
    this.textures.delete(id);
  }

  private createPlaceholderTexture(color: readonly [number, number, number]): WebGLTexture {
    const gl = this.gl;
    const tex = gl.createTexture();
    if (!tex) throw new Error("marquee: createTexture failed");
    gl.bindTexture(gl.TEXTURE_2D, tex);
    const [r, g, b] = color;
    gl.texImage2D(
      gl.TEXTURE_2D, 0, gl.RGB, 1, 1, 0, gl.RGB, gl.UNSIGNED_BYTE,
      new Uint8Array([Math.round(r * 255), Math.round(g * 255), Math.round(b * 255)]),
    );
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    return tex;
  }
}
